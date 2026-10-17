package rules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/lwl0820/porter/internal/auth"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/quota"
	"github.com/lwl0820/porter/internal/store"
)

// AccountRepository 与规则持久化共享服务操作锁，避免分配和规则占用交错。
type AccountRepository interface {
	ListUsers(context.Context) ([]model.User, error)
	ListPlans(context.Context) ([]model.Plan, error)
	CreatePlan(context.Context, model.Plan) error
	Settings(context.Context) (model.Settings, error)
	SaveSettings(context.Context, model.Settings) error
	CreateUser(context.Context, model.User, model.Usage, int) (model.User, error)
	ListUsages(context.Context) ([]model.Usage, error)
	SaveUsages(context.Context, []model.Usage) error
	ResetUsage(context.Context, model.Usage) error
}

func identifier() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func (s *Service) loadAccounts(ctx context.Context) error {
	users, err := s.accounts.ListUsers(ctx)
	if err != nil {
		return err
	}
	plans, err := s.accounts.ListPlans(ctx)
	if err != nil {
		return err
	}
	usages, err := s.accounts.ListUsages(ctx)
	if err != nil {
		return err
	}
	for _, p := range plans {
		s.plans[p.ID] = p
	}
	for _, u := range users {
		s.users[u.ID] = u
		if u.Role == model.RoleUser && s.adminPort >= u.PortStart && s.adminPort <= u.PortEnd {
			return errors.New("管理端口与用户分配的数字端口冲突")
		}
	}
	for _, v := range usages {
		u, ok := s.users[v.UserID]
		if !ok || u.PlanID == nil {
			return errors.New("用户用量缺少所属账号或套餐")
		}
		p, ok := s.plans[*u.PlanID]
		if !ok {
			return errors.New("用户套餐不存在")
		}
		m := newUserMeter(u, p, v, s.now, s.signalQuota)
		s.meters[u.ID] = m
		s.persistedUsage[u.ID] = v
		if next, ok := m.pendingUsage(); ok {
			if err := s.accounts.ResetUsage(ctx, next); err != nil {
				slog.Error("启动时用户周期推进失败，将单独暂停并重试", "user_id", u.ID, "error", err)
				continue
			}
			m.commitCycle(next)
			s.persistedUsage[u.ID] = next
		}
	}
	for _, u := range users {
		if u.Role == model.RoleUser && s.meters[u.ID] == nil {
			return errors.New("普通用户缺少用量账本，无法安全恢复转发")
		}
	}
	return nil
}

// validateOwnerPort 不推进周期，供启动及周期恢复在准入前验证配置。
func (s *Service) validateOwnerPort(r model.Rule) error {
	if s.accounts == nil {
		return nil
	}
	u, ok := s.users[r.OwnerID]
	if !ok {
		return problem(404, "not_found", "所属用户不存在")
	}
	if u.Role == model.RoleUser {
		if r.ListenPort < u.PortStart || r.ListenPort > u.PortEnd {
			return &Error{Status: 400, Code: "validation_error", Message: "监听端口超出用户分配范围", Fields: map[string]string{"listen_port": "请选择用户分配范围内的端口"}}
		}
	} else {
		for _, other := range s.users {
			if other.Role == model.RoleUser && r.ListenPort >= other.PortStart && r.ListenPort <= other.PortEnd {
				return problem(409, "port_reserved", "数字端口已分配给用户")
			}
		}
	}
	return nil
}
func (s *Service) validateOwner(r model.Rule) error {
	if err := s.validateOwnerPort(r); err != nil {
		return err
	}
	if s.accounts == nil || s.users[r.OwnerID].Role != model.RoleUser {
		return nil
	}
	m := s.meters[r.OwnerID]
	if m == nil {
		return problem(500, "database_error", "用户用量不存在")
	}
	if _, pending := m.pendingUsage(); pending {
		s.coordinateQuota(context.Background())
	}
	if r.Enabled && (m.Snapshot().Exceeded || m.pending()) {
		return problem(409, "quota_exceeded", "用户流量额度已耗尽或正在重置")
	}
	return nil
}
func (s *Service) authorize(actor model.User, id string) error {
	if s.closed {
		return problem(503, "shutting_down", "服务正在停止")
	}
	r, ok := s.items[id]
	if !ok || (actor.Role != model.RoleAdmin && r.OwnerID != actor.ID) {
		return problem(404, "not_found", "规则不存在")
	}
	if actor.Role != model.RoleAdmin && actor.Role != model.RoleUser {
		return problem(403, "forbidden", "无权操作")
	}
	return nil
}
func (s *Service) ruleFor(actor model.User, r model.Rule) model.Rule {
	r = s.view(r)
	r.Owner = nil
	if actor.Role == model.RoleAdmin {
		if u, ok := s.users[r.OwnerID]; ok {
			o := u.Owner()
			r.Owner = &o
		}
	}
	return r
}
func (s *Service) ListFor(actor model.User, owner string) []model.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]model.Rule, 0)
	for _, r := range s.items {
		if actor.Role == model.RoleAdmin {
			if owner != "" && r.OwnerID != owner {
				continue
			}
		} else if actor.Role != model.RoleUser || r.OwnerID != actor.ID {
			continue
		}
		result = append(result, s.ruleFor(actor, r))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// GetFor 在与写操作相同的锁下校验权限并取得运行态详情。
func (s *Service) GetFor(actor model.User, id string) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorize(actor, id); err != nil {
		return model.Rule{}, err
	}
	return s.ruleFor(actor, s.items[id]), nil
}

func (s *Service) SaveFor(ctx context.Context, actor model.User, owner, id string, revision int64, input model.Input) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return model.Rule{}, problem(503, "shutting_down", "服务正在停止")
	}
	if actor.Role != model.RoleAdmin && actor.Role != model.RoleUser {
		return model.Rule{}, problem(403, "forbidden", "无权操作")
	}
	if id != "" {
		if err := s.authorize(actor, id); err != nil {
			return model.Rule{}, err
		}
	} else if actor.Role != model.RoleAdmin {
		owner = actor.ID
	} else if owner == "" {
		owner = actor.ID
	}
	r, err := s.saveLocked(ctx, id, revision, input, owner)
	if err != nil {
		return r, err
	}
	auditRule("保存", actor, r)
	return s.ruleFor(actor, r), nil
}
func (s *Service) DeleteFor(ctx context.Context, actor model.User, id string, revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorize(actor, id); err != nil {
		return err
	}
	r := s.items[id]
	if err := s.deleteLocked(ctx, id, revision); err != nil {
		return err
	}
	auditRule("删除", actor, r)
	return nil
}
func (s *Service) RetryFor(actor model.User, id string, revision int64) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorize(actor, id); err != nil {
		return model.Rule{}, err
	}
	r, err := s.retryLocked(id, revision)
	if err != nil {
		return r, err
	}
	auditRule("重试", actor, r)
	return s.ruleFor(actor, r), nil
}
func (s *Service) ResetTrafficFor(ctx context.Context, actor model.User, id string, revision int64) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.authorize(actor, id); err != nil {
		return model.Rule{}, err
	}
	r, err := s.resetTrafficLocked(ctx, id, revision)
	if err != nil {
		return r, err
	}
	auditRule("重置展示统计", actor, r)
	return s.ruleFor(actor, r), nil
}
func (s *Service) requireAdmin(actor model.User) error {
	if s.closed {
		return problem(503, "shutting_down", "服务正在停止")
	}
	if actor.Role != model.RoleAdmin {
		return problem(403, "forbidden", "仅管理员可操作")
	}
	if s.accounts == nil {
		return problem(503, "unavailable", "账号存储不可用")
	}
	return nil
}
func (s *Service) Plans(actor model.User) ([]model.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	result := make([]model.Plan, 0, len(s.plans))
	for _, p := range s.plans {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}
func (s *Service) CreatePlan(ctx context.Context, actor model.User, in model.PlanInput) (model.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireAdmin(actor); err != nil {
		return model.Plan{}, err
	}
	fields := map[string]string{}
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 80 || !utf8.ValidString(in.Name) {
		fields["name"] = "名称需要 1–80 个字符"
	}
	if in.PortCount < 1 || in.PortCount > 65535 {
		fields["port_count"] = "端口数量范围为 1–65535"
	}
	if in.TrafficLimitBytes <= 0 {
		fields["traffic_limit_bytes"] = "流量额度必须大于零"
	}
	switch in.AccountingMode {
	case model.AccountingBoth, model.AccountingMax, model.AccountingUpload, model.AccountingDownload:
	default:
		fields["accounting_mode"] = "请选择有效计费方式"
	}
	switch in.ResetMode {
	case model.ResetCalendarMonth, model.ResetAnniversary:
		if in.ResetDay != nil {
			fields["reset_day"] = "该重置方式不接受固定日"
		}
	case model.ResetFixedDay:
		if in.ResetDay == nil || *in.ResetDay < 1 || *in.ResetDay > 31 {
			fields["reset_day"] = "固定日范围为 1–31"
		}
	default:
		fields["reset_mode"] = "请选择有效重置方式"
	}
	if len(fields) > 0 {
		return model.Plan{}, &Error{Status: 400, Code: "validation_error", Message: "请检查套餐配置", Fields: fields}
	}
	id, err := identifier()
	if err != nil {
		return model.Plan{}, err
	}
	p := model.Plan{ID: id, Name: in.Name, PortCount: in.PortCount, TrafficLimitBytes: in.TrafficLimitBytes, AccountingMode: in.AccountingMode, ResetMode: in.ResetMode, ResetDay: in.ResetDay, CreatedAt: s.now().UTC()}
	if err = s.accounts.CreatePlan(ctx, p); err != nil {
		return model.Plan{}, accountError(err)
	}
	s.plans[id] = p
	return p, nil
}
func (s *Service) accountLocked(id string) (model.Account, error) {
	u, ok := s.users[id]
	if !ok {
		return model.Account{}, problem(404, "not_found", "用户不存在")
	}
	a := model.Account{User: u}
	if u.PlanID != nil {
		p := s.plans[*u.PlanID]
		a.Plan = &p
		if m := s.meters[id]; m != nil {
			v := m.Snapshot()
			a.Usage = &v
		}
	}
	return a, nil
}
func (s *Service) Account(actor model.User) (model.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coordinateQuota(context.Background())
	return s.accountLocked(actor.ID)
}
func (s *Service) Users(actor model.User) ([]model.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireAdmin(actor); err != nil {
		return nil, err
	}
	s.coordinateQuota(context.Background())
	result := make([]model.Account, 0)
	for id, u := range s.users {
		if u.Role != model.RoleUser {
			continue
		}
		a, err := s.accountLocked(id)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].User.CreatedAt.Before(result[j].User.CreatedAt) })
	return result, nil
}
func (s *Service) CreateUser(ctx context.Context, actor model.User, in model.UserInput) (model.Account, error) {
	// 密码派生不持有服务锁，账号和端口事务仍由统一锁串行。
	in.Username = strings.TrimSpace(in.Username)
	fields := map[string]string{}
	if n := utf8.RuneCountInString(in.Username); n < 1 || n > 80 || !utf8.ValidString(in.Username) {
		fields["username"] = "用户名需要 1–80 个字符"
	}
	if strings.TrimSpace(in.Password) == "" || len(in.Password) < 8 || len(in.Password) > 1024 {
		fields["password"] = "密码需要 8–1024 字节"
	}
	if len(fields) > 0 {
		return model.Account{}, &Error{Status: 400, Code: "validation_error", Message: "请检查用户信息", Fields: fields}
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return model.Account{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireAdmin(actor); err != nil {
		return model.Account{}, err
	}
	p, ok := s.plans[in.PlanID]
	if !ok {
		return model.Account{}, &Error{Status: 400, Code: "validation_error", Message: "套餐不存在", Fields: map[string]string{"plan_id": "请选择已有套餐"}}
	}
	id, err := identifier()
	if err != nil {
		return model.Account{}, err
	}
	u := model.User{ID: id, Username: in.Username, Role: model.RoleUser, PlanID: &in.PlanID, PasswordHash: hash, CreatedAt: s.now().UTC()}
	start, end := quota.Cycle(p, u, u.CreatedAt)
	v := model.Usage{UserID: id, CycleStart: start, CycleEnd: end, LimitBytes: p.TrafficLimitBytes, Version: 1}
	u, err = s.accounts.CreateUser(ctx, u, v, s.adminPort)
	if err != nil {
		return model.Account{}, accountError(err)
	}
	s.users[id] = u
	s.meters[id] = newUserMeter(u, p, v, s.now, s.signalQuota)
	s.persistedUsage[id] = v
	return s.accountLocked(id)
}
func (s *Service) Settings(ctx context.Context, actor model.User) (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireAdmin(actor); err != nil {
		return model.Settings{}, err
	}
	v, err := s.accounts.Settings(ctx)
	if err != nil {
		return v, accountError(err)
	}
	return v, nil
}
func (s *Service) SaveSettings(ctx context.Context, actor model.User, revision int64, in model.SettingsInput) (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireAdmin(actor); err != nil {
		return model.Settings{}, err
	}
	if in.PortStart < 1 || in.PortEnd > 65535 || in.PortStart > in.PortEnd {
		return model.Settings{}, &Error{Status: 400, Code: "validation_error", Message: "请检查端口范围", Fields: map[string]string{"port_start": "范围需要满足 1 ≤ 起始 ≤ 结束 ≤ 65535"}}
	}
	v := model.Settings{PortStart: in.PortStart, PortEnd: in.PortEnd, Revision: revision + 1}
	if err := s.accounts.SaveSettings(ctx, v); err != nil {
		return model.Settings{}, accountError(err)
	}
	return v, nil
}
func accountError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return problem(404, "not_found", "资源不存在")
	case errors.Is(err, store.ErrConflict):
		return problem(409, "name_conflict", "名称已存在")
	case errors.Is(err, store.ErrPortPoolUnset):
		return problem(409, "port_pool_unset", "请先配置用户端口分配范围")
	case errors.Is(err, store.ErrPortExhausted):
		return problem(409, "port_exhausted", "没有足够的连续可用端口")
	case errors.Is(err, store.ErrRevision):
		return problem(412, "revision_mismatch", "配置已被修改，请刷新后重试")
	default:
		return problem(500, "database_error", "数据库操作失败")
	}
}

func auditRule(action string, actor model.User, r model.Rule) {
	slog.Info("规则操作完成", "action", action, "actor_id", actor.ID, "owner_id", r.OwnerID, "rule_id", r.ID)
}
