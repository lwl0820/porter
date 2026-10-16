package rules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
)

type Repository interface {
	List(context.Context) ([]model.Rule, error)
	Save(context.Context, model.Rule, bool) error
	Delete(context.Context, string) error
}
type Error struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Message }
func problem(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

type Service struct {
	mu        sync.Mutex
	repo      Repository
	engine    *forward.Engine
	items     map[string]model.Rule
	running   map[string]*forward.Runtime
	adminHost netip.Addr
	adminPort int
	closed    bool
}

func New(ctx context.Context, repo Repository, engine *forward.Engine, adminAddr string) (*Service, error) {
	h, p, err := net.SplitHostPort(adminAddr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return nil, err
	}
	addr, err := netip.ParseAddr(h)
	if h == "" {
		addr = netip.IPv4Unspecified()
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("管理监听地址必须是 IP：%w", err)
	}
	s := &Service{repo: repo, engine: engine, items: make(map[string]model.Rule), running: make(map[string]*forward.Runtime), adminHost: addr.Unmap(), adminPort: port}
	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range items {
		s.items[r.ID] = r
	}
	for _, r := range items {
		if !r.Enabled {
			continue
		}
		input := model.Input{Name: r.Name, Protocol: r.Protocol, ListenHost: r.ListenHost, ListenPort: r.ListenPort, TargetHost: r.TargetHost, TargetPort: r.TargetPort, Enabled: &r.Enabled}
		if _, err = s.validate(input, r.ID); err != nil {
			s.setError(r, err)
			continue
		}
		rt, e := engine.Prepare(r)
		if e != nil {
			s.setError(r, e)
			continue
		}
		s.running[r.ID] = rt
		rt.Start()
	}
	return s, nil
}

func (s *Service) setError(r model.Rule, err error) {
	r.Status = "error"
	r.LastErrorCode = "listen_failed"
	r.LastError = "监听失败，请检查地址、端口占用和权限"
	s.items[r.ID] = r
	slog.Error("规则恢复失败", "rule_id", r.ID, "error", err)
}

func (s *Service) List() []model.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]model.Rule, 0, len(s.items))
	for _, r := range s.items {
		items = append(items, s.view(r))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

func (s *Service) view(r model.Rule) model.Rule {
	if !r.Enabled {
		r.Status = "disabled"
		r.LastError = ""
		r.LastErrorCode = ""
		return r
	}
	if rt := s.running[r.ID]; rt != nil {
		if err := rt.Error(); err == nil {
			r.Status = "running"
			r.LastError = ""
			r.LastErrorCode = ""
		} else {
			r.Status = "error"
			r.LastErrorCode = "listen_failed"
			r.LastError = "监听器异常，请重试"
		}
	}
	if r.Status == "" {
		r.Status = "error"
		r.LastErrorCode = "listen_failed"
		r.LastError = "监听未启动，请重试"
	}
	return r
}

func (s *Service) Save(ctx context.Context, id string, revision int64, input model.Input) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return model.Rule{}, problem(503, "shutting_down", "服务正在停止")
	}
	create := id == ""
	old, exists := s.items[id]
	if !create {
		if !exists {
			return model.Rule{}, problem(404, "not_found", "规则不存在")
		}
		if revision != old.Revision {
			return model.Rule{}, problem(412, "revision_mismatch", "规则已被修改，请刷新后重试")
		}
	}
	if create && len(s.items) >= 1000 {
		return model.Rule{}, problem(409, "rule_limit", "最多保存 1000 条规则")
	}
	r, err := s.validate(input, id)
	if err != nil {
		return model.Rule{}, err
	}
	now := time.Now().UTC()
	r.ID = id
	r.Revision = old.Revision + 1
	r.CreatedAt = old.CreatedAt
	r.UpdatedAt = now
	if create {
		var b [16]byte
		if _, err = rand.Read(b[:]); err != nil {
			return model.Rule{}, err
		}
		r.ID = hex.EncodeToString(b[:])
		r.CreatedAt = now
	}
	rt := s.running[id]
	same := rt != nil && rt.Error() == nil && old.Protocol == r.Protocol && old.ListenHost == r.ListenHost && old.ListenPort == r.ListenPort
	var prepared *forward.Runtime
	if r.Enabled && !same {
		prepared, err = s.engine.Prepare(r)
		if err != nil {
			slog.Warn("规则监听准备失败", "rule_id", id, "error", err)
			return model.Rule{}, problem(409, "listen_conflict", "无法监听此地址和端口，请检查占用、权限；更改重叠地址时请先停用规则")
		}
	}
	if err = s.repo.Save(ctx, r, create); err != nil {
		if prepared != nil {
			prepared.Stop()
		}
		slog.Error("规则保存失败", "rule_id", id, "error", err)
		return model.Rule{}, problem(500, "database_error", "规则保存失败，原配置保持不变")
	}
	if r.Enabled {
		if prepared != nil {
			if rt != nil {
				rt.Stop()
			}
			s.running[r.ID] = prepared
			prepared.Start()
		} else if old.TargetHost != r.TargetHost || old.TargetPort != r.TargetPort {
			rt.Update(r)
		}
	} else {
		if rt != nil {
			rt.Stop()
		}
		delete(s.running, id)
	}
	s.items[r.ID] = r
	slog.Info("规则已保存", "rule_id", r.ID, "enabled", r.Enabled, "revision", r.Revision)
	return s.view(r), nil
}

func (s *Service) Delete(ctx context.Context, id string, revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return problem(503, "shutting_down", "服务正在停止")
	}
	r, exists := s.items[id]
	if !exists {
		return problem(404, "not_found", "规则不存在")
	}
	if revision != r.Revision {
		return problem(412, "revision_mismatch", "规则已被修改，请刷新后重试")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		slog.Error("删除规则失败", "rule_id", id, "error", err)
		return problem(500, "database_error", "删除失败，规则保持不变")
	}
	if rt := s.running[id]; rt != nil {
		rt.Stop()
	}
	delete(s.running, id)
	delete(s.items, id)
	slog.Info("规则已删除", "rule_id", id)
	return nil
}

func (s *Service) Retry(id string, revision int64) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return model.Rule{}, problem(503, "shutting_down", "服务正在停止")
	}
	r, ok := s.items[id]
	if !ok {
		return model.Rule{}, problem(404, "not_found", "规则不存在")
	}
	if r.Revision != revision {
		return model.Rule{}, problem(412, "revision_mismatch", "规则已被修改，请刷新后重试")
	}
	if !r.Enabled {
		return model.Rule{}, problem(409, "disabled", "请先启用规则")
	}
	if rt := s.running[id]; rt != nil {
		if rt.Error() == nil {
			return s.view(r), nil
		}
		rt.Stop()
		delete(s.running, id)
	}
	rt, err := s.engine.Prepare(r)
	if err != nil {
		s.setError(r, err)
		return s.view(s.items[id]), problem(409, "listen_conflict", "重试失败，请检查地址、端口占用和权限")
	}
	s.running[id] = rt
	rt.Start()
	return s.view(r), nil
}

func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for id, rt := range s.running {
		rt.Stop()
		delete(s.running, id)
	}
}

func (s *Service) validate(in model.Input, id string) (model.Rule, error) {
	r := model.Rule{Name: strings.TrimSpace(in.Name), Protocol: strings.ToLower(strings.TrimSpace(in.Protocol)), ListenHost: strings.TrimSpace(in.ListenHost), ListenPort: in.ListenPort, TargetHost: strings.TrimSuffix(strings.TrimSpace(in.TargetHost), "."), TargetPort: in.TargetPort, Enabled: true}
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	}
	fields := map[string]string{}
	if n := utf8.RuneCountInString(r.Name); n < 1 || n > 80 || !utf8.ValidString(r.Name) {
		fields["name"] = "名称需要 1–80 个字符"
	}
	if r.Protocol != "tcp" && r.Protocol != "udp" {
		fields["protocol"] = "请选择 TCP 或 UDP"
	}
	a, err := netip.ParseAddr(r.ListenHost)
	if err != nil || a.Zone() != "" {
		fields["listen_host"] = "请输入本机 IPv4 或 IPv6 地址"
	} else {
		a = a.Unmap()
		r.ListenHost = a.String()
	}
	if r.ListenPort < 1 || r.ListenPort > 65535 {
		fields["listen_port"] = "端口范围为 1–65535"
	}
	if r.TargetPort < 1 || r.TargetPort > 65535 {
		fields["target_port"] = "端口范围为 1–65535"
	}
	if target, e := netip.ParseAddr(r.TargetHost); e == nil {
		if target.IsUnspecified() || target.IsMulticast() || target.Zone() != "" {
			fields["target_host"] = "目标必须是单播地址或域名"
		} else {
			r.TargetHost = target.Unmap().String()
		}
	} else {
		r.TargetHost = strings.ToLower(r.TargetHost)
		if !domain(r.TargetHost) {
			fields["target_host"] = "请输入 IP 或域名，不包含协议、路径或端口"
		}
	}
	if len(fields) > 0 {
		return r, &Error{Status: 400, Code: "validation_error", Message: "请检查表单内容", Fields: fields}
	}
	if r.Protocol == "tcp" && r.ListenPort == s.adminPort && overlap(a, s.adminHost) {
		return r, problem(409, "admin_conflict", "监听地址与面板管理端口冲突")
	}
	for otherID, other := range s.items {
		if otherID == id || other.Protocol != r.Protocol || other.ListenPort != r.ListenPort {
			continue
		}
		b, e := netip.ParseAddr(other.ListenHost)
		if e == nil && overlap(a, b) {
			return r, problem(409, "rule_conflict", "此协议的监听地址和端口已被规则使用")
		}
	}
	if target, e := netip.ParseAddr(r.TargetHost); e == nil && r.ListenPort == r.TargetPort && localAddress(target) && overlap(a, target) {
		return r, problem(400, "forward_loop", "目标不能指向规则自身的监听地址")
	}
	return r, nil
}

func overlap(a, b netip.Addr) bool {
	a = a.Unmap()
	b = b.Unmap()
	return a.Is4() == b.Is4() && (a == b || a.IsUnspecified() || b.IsUnspecified())
}
func localAddress(a netip.Addr) bool {
	if a.IsLoopback() {
		return true
	}
	addrs, _ := net.InterfaceAddrs()
	for _, v := range addrs {
		p, e := netip.ParsePrefix(v.String())
		if e == nil && p.Addr().Unmap() == a.Unmap() {
			return true
		}
	}
	return false
}
func domain(v string) bool {
	if len(v) < 1 || len(v) > 253 {
		return false
	}
	for _, label := range strings.Split(v, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
