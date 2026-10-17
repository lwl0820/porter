package rules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	SaveTraffic(context.Context, map[string]model.Traffic) error
	ResetTraffic(context.Context, model.Rule) error
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
	mu             sync.Mutex
	repo           Repository
	engine         *forward.Engine
	items          map[string]model.Rule
	running        map[string]*runtimeGroup
	monitors       sync.WaitGroup
	adminHost      netip.Addr
	adminPort      int
	closed         bool
	traffic        map[string]*forward.Traffic
	persisted      map[string]model.Traffic
	stopTraffic    chan struct{}
	trafficDone    chan struct{}
	closeOnce      sync.Once
	closeErr       error
	accounts       AccountRepository
	users          map[string]model.User
	plans          map[string]model.Plan
	meters         map[string]*userMeter
	quotaEvents    chan struct{}
	flushInterval  time.Duration
	persistedUsage map[string]model.Usage
	now            func() time.Time
}

type Options struct {
	FlushInterval time.Duration
	Now           func() time.Time
}

func New(ctx context.Context, repo Repository, engine *forward.Engine, adminAddr string, options ...Options) (*Service, error) {
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
	s := &Service{repo: repo, engine: engine, items: make(map[string]model.Rule), running: make(map[string]*runtimeGroup), adminHost: addr.Unmap(), adminPort: port, traffic: make(map[string]*forward.Traffic), persisted: make(map[string]model.Traffic), stopTraffic: make(chan struct{}), trafficDone: make(chan struct{})}
	s.flushInterval = 5 * time.Second
	s.now = time.Now
	if len(options) > 0 {
		if options[0].FlushInterval > 0 {
			s.flushInterval = options[0].FlushInterval
		}
		if options[0].Now != nil {
			s.now = options[0].Now
		}
	}
	s.users = make(map[string]model.User)
	s.plans = make(map[string]model.Plan)
	s.meters = make(map[string]*userMeter)
	s.persistedUsage = make(map[string]model.Usage)
	s.quotaEvents = make(chan struct{}, 1)
	if accountRepo, ok := repo.(AccountRepository); ok {
		s.accounts = accountRepo
		if err := s.loadAccounts(ctx); err != nil {
			return nil, err
		}
	}
	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range items {
		s.items[r.ID] = r
		s.traffic[r.ID] = forward.NewTraffic(r.TrafficEnabled, r.Traffic, time.Now())
		s.persisted[r.ID] = r.Traffic
	}
	s.mu.Lock()
	for _, r := range items {
		if !r.Enabled {
			continue
		}
		input := model.Input{Name: r.Name, Protocol: r.Protocol, ListenHost: r.ListenHost, ListenPort: r.ListenPort, TargetHost: r.TargetHost, TargetPort: r.TargetPort, Enabled: &r.Enabled, TrafficEnabled: &r.TrafficEnabled}
		if _, err = s.validate(input, r.ID); err != nil {
			s.setError(r, err)
			continue
		}
		if err := s.validateOwnerPort(r); err != nil {
			s.setError(r, err)
			continue
		}
		if meter := s.meters[r.OwnerID]; meter != nil {
			if _, pending := meter.pendingUsage(); pending {
				r.Status = "error"
				r.LastErrorCode = "quota_reset_pending"
				r.LastError = "流量周期重置暂未完成，正在重试"
				s.items[r.ID] = r
				continue
			}
			if meter.Snapshot().Exceeded {
				r.Status = "quota_exceeded"
				s.items[r.ID] = r
				continue
			}
		}

		group, e := s.prepareGroup(r, nil)
		if e != nil {
			s.setError(r, e)
			continue
		}
		s.startGroup(r.ID, group)
	}
	s.mu.Unlock()
	go s.trafficLoop()
	return s, nil
}

func (s *Service) setError(r model.Rule, err error) {
	r.Status = "error"
	r.LastErrorCode = "listen_failed"
	r.LastError = listenerMessage(err)
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

func (s *Service) view(r model.Rule) (result model.Rule) {
	defer func() {
		if t := s.traffic[result.ID]; t != nil {
			result.Traffic = t.Snapshot()
		}
		if !result.TrafficEnabled || result.Status != "running" {
			result.Traffic.UploadBytesPerSecond = 0
			result.Traffic.DownloadBytesPerSecond = 0
		}
	}()
	if !r.Enabled {
		r.Status = "disabled"
		r.LastError = ""
		r.LastErrorCode = ""
		return r
	}
	if meter := s.meters[r.OwnerID]; meter != nil && r.LastErrorCode == "quota_reset_pending" && meter.pending() {
		return r
	}
	if meter := s.meters[r.OwnerID]; meter != nil && meter.Snapshot().Exceeded {
		r.Status = "quota_exceeded"
		r.LastError = ""
		r.LastErrorCode = ""
		return r
	}
	if group := s.running[r.ID]; group != nil {
		if err := group.failure(); err == nil {
			r.Status = "running"
			r.LastError = ""
			r.LastErrorCode = ""
		} else {
			s.failGroup(r, group, err)
			r = s.items[r.ID]
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
	return s.saveLocked(ctx, id, revision, input, model.AdminID)
}

func (s *Service) saveLocked(ctx context.Context, id string, revision int64, input model.Input, ownerID string) (model.Rule, error) {
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
	effectiveOwner := ownerID
	if !create {
		effectiveOwner = old.OwnerID
	}
	if err := s.validateOwner(model.Rule{OwnerID: effectiveOwner, ListenPort: input.ListenPort, Enabled: input.Enabled == nil || *input.Enabled}); err != nil {
		return model.Rule{}, err
	}
	r, err := s.validate(input, id)
	if err != nil {
		return model.Rule{}, err
	}
	if input.TrafficEnabled == nil {
		r.TrafficEnabled = old.TrafficEnabled
	}
	r.OwnerID = ownerID
	if !create {
		r.OwnerID = old.OwnerID
	}
	if err := s.validateOwner(r); err != nil {
		return model.Rule{}, err
	}
	now := s.now().UTC()
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
	committed := false
	if create {
		s.traffic[r.ID] = forward.NewTraffic(false, model.Traffic{}, now)
		defer func() {
			if !committed {
				delete(s.traffic, r.ID)
			}
		}()
	}
	previous := s.running[id]
	if previous != nil {
		if fault := previous.failure(); fault != nil {
			s.failGroup(old, previous, fault)
			previous = nil
		}
	}
	var reusable *runtimeGroup
	if previous != nil && old.ListenHost == r.ListenHost && old.ListenPort == r.ListenPort {
		reusable = previous
	}
	var prepared *runtimeGroup
	if r.Enabled {
		prepared, err = s.prepareGroup(r, reusable)
		if err != nil {
			slog.Warn("规则监听准备失败", "rule_id", id, "error", err)
			return model.Rule{}, problem(409, "listen_conflict", listenerMessage(err)+"；更改重叠地址时请先停用规则")
		}
	}
	if err = s.repo.Save(ctx, r, create); err != nil {
		releaseAdded(prepared, previous)
		slog.Error("规则保存失败", "rule_id", id, "error", err)
		return model.Rule{}, problem(500, "database_error", "规则保存失败，原配置保持不变")
	}
	committed = true
	s.traffic[r.ID].SetEnabled(r.TrafficEnabled, time.Now())
	if previous != nil {
		previous.retire()
		for transport, rt := range previous.members {
			if prepared == nil || prepared.members[transport] != rt {
				rt.Stop()
			}
		}
	}
	delete(s.running, id)
	s.items[r.ID] = r
	if prepared != nil {
		reset := old.Protocol != r.Protocol || old.TargetHost != r.TargetHost || old.TargetPort != r.TargetPort
		if reset && previous != nil {
			for transport, rt := range prepared.members {
				if previous.members[transport] == rt {
					rt.Update(physicalRule(r, transport))
				}
			}
		}
		s.startGroup(r.ID, prepared)
	}
	slog.Info("规则已保存", "rule_id", r.ID, "enabled", r.Enabled, "revision", r.Revision)
	return s.view(r), nil
}

func (s *Service) Delete(ctx context.Context, id string, revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return problem(503, "shutting_down", "服务正在停止")
	}
	return s.deleteLocked(ctx, id, revision)
}

func (s *Service) deleteLocked(ctx context.Context, id string, revision int64) error {
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
	if group := s.running[id]; group != nil {
		group.stop()
	}
	delete(s.running, id)
	delete(s.items, id)
	delete(s.traffic, id)
	delete(s.persisted, id)
	slog.Info("规则已删除", "rule_id", id)
	return nil
}

func (s *Service) Retry(id string, revision int64) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return model.Rule{}, problem(503, "shutting_down", "服务正在停止")
	}
	return s.retryLocked(id, revision)
}

func (s *Service) retryLocked(id string, revision int64) (model.Rule, error) {
	r, ok := s.items[id]
	if !ok {
		return model.Rule{}, problem(404, "not_found", "规则不存在")
	}
	if r.Revision != revision {
		return model.Rule{}, problem(412, "revision_mismatch", "规则已被修改，请刷新后重试")
	}
	if err := s.validateOwner(r); err != nil {
		return model.Rule{}, err
	}
	if !r.Enabled {
		return model.Rule{}, problem(409, "disabled", "请先启用规则")
	}
	if group := s.running[id]; group != nil {
		if group.failure() == nil {
			return s.view(r), nil
		}
		group.stop()
		delete(s.running, id)
	}
	in := model.Input{Name: r.Name, Protocol: r.Protocol, ListenHost: r.ListenHost, ListenPort: r.ListenPort, TargetHost: r.TargetHost, TargetPort: r.TargetPort, Enabled: &r.Enabled, TrafficEnabled: &r.TrafficEnabled}
	if _, err := s.validate(in, id); err != nil {
		return model.Rule{}, err
	}
	group, err := s.prepareGroup(r, nil)
	if err != nil {
		s.setError(r, err)
		return s.view(s.items[id]), problem(409, "listen_conflict", listenerMessage(err))
	}
	s.startGroup(id, group)
	return s.view(r), nil
}

func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		if s.stopTraffic != nil {
			close(s.stopTraffic)
		}
		s.mu.Unlock()
		if s.trafficDone != nil {
			<-s.trafficDone
		}
		s.mu.Lock()
		for id, group := range s.running {
			group.stop()
			delete(s.running, id)
		}
		s.closeErr = errors.Join(s.settleCycles(context.Background()), s.flushTraffic(context.Background()), s.flushUsages(context.Background()))
		s.mu.Unlock()
		s.monitors.Wait()
		if s.closeErr != nil {
			slog.Error("退出时流量统计保存失败", "error", s.closeErr)
		}
	})
	return s.closeErr
}

func (s *Service) validate(in model.Input, id string) (model.Rule, error) {
	r := model.Rule{Name: strings.TrimSpace(in.Name), Protocol: strings.ToLower(strings.TrimSpace(in.Protocol)), ListenHost: strings.TrimSpace(in.ListenHost), ListenPort: in.ListenPort, TargetHost: strings.TrimSuffix(strings.TrimSpace(in.TargetHost), "."), TargetPort: in.TargetPort, Enabled: true}
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	}
	if in.TrafficEnabled != nil {
		r.TrafficEnabled = *in.TrafficEnabled
	}
	fields := map[string]string{}
	if n := utf8.RuneCountInString(r.Name); n < 1 || n > 80 || !utf8.ValidString(r.Name) {
		fields["name"] = "名称需要 1–80 个字符"
	}
	if len(model.Transports(r.Protocol)) == 0 {
		fields["protocol"] = "请选择 TCP、UDP 或 TCP+UDP"
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
	if model.HasTransport(r.Protocol, model.ProtocolTCP) && r.ListenPort == s.adminPort && overlap(a, s.adminHost) {
		return r, problem(409, "admin_conflict", "TCP 监听地址与面板管理端口冲突")
	}
	for otherID, other := range s.items {
		common := model.CommonTransport(r.Protocol, other.Protocol)
		if otherID == id || common == "" || other.ListenPort != r.ListenPort {
			continue
		}
		b, e := netip.ParseAddr(other.ListenHost)
		if e == nil && overlap(a, b) {
			return r, problem(409, "rule_conflict", strings.ToUpper(common)+" 监听地址和端口已被规则使用")
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
