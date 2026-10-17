package rules

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/quota"
)

// userMeter 的短锁与服务操作锁独立；转发协程不能同步停止自己。
type userMeter struct {
	mu     sync.Mutex
	user   model.User
	plan   model.Plan
	value  model.Usage
	next   *model.Usage
	ready  chan struct{}
	now    func() time.Time
	notify func()
}

var errQuota = errors.New("用户流量额度已耗尽")

func newUserMeter(u model.User, p model.Plan, v model.Usage, now func() time.Time, notify func()) *userMeter {
	m := &userMeter{user: u, plan: p, value: v, now: now, notify: notify}
	m.update(&m.value)
	return m
}
func (m *userMeter) update(v *model.Usage) {
	v.LimitBytes = m.plan.TrafficLimitBytes
	v.UsedBytes = quota.Used(m.plan.AccountingMode, v.UploadBytes, v.DownloadBytes)
	v.Exceeded = v.UsedBytes >= v.LimitBytes
}

// advance 不把尚未提交的额度公开。边界后已完成的在途写入进入下一周期暂存。
func (m *userMeter) advance(now time.Time) {
	current := &m.value
	if m.next != nil {
		current = m.next
	}
	if now.Before(current.CycleEnd) {
		return
	}
	start, end := quota.Cycle(m.plan, m.user, now)
	if !start.After(current.CycleStart) {
		return
	}
	m.next = &model.Usage{UserID: m.user.ID, CycleStart: start, CycleEnd: end, Version: m.value.Version + 1, LimitBytes: m.plan.TrafficLimitBytes}
	if m.ready == nil {
		m.ready = make(chan struct{})
	}
	m.notify()
}
func (m *userMeter) Wait(ctx context.Context) error {
	for {
		m.mu.Lock()
		m.advance(m.now())
		if m.next != nil {
			ready := m.ready
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ready:
				continue
			}
		}
		exceeded := m.value.Exceeded
		m.mu.Unlock()
		if exceeded {
			return errQuota
		}
		return nil
	}
}
func (m *userMeter) Add(upload bool, n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(m.now())
	v := &m.value
	if m.next != nil {
		v = m.next
	}
	if upload {
		v.UploadBytes = quota.SaturatingAdd(v.UploadBytes, int64(n))
	} else {
		v.DownloadBytes = quota.SaturatingAdd(v.DownloadBytes, int64(n))
	}
	was := v.Exceeded
	m.update(v)
	if v.Exceeded && !was {
		m.notify()
	}
}
func (m *userMeter) Snapshot() model.Usage { m.mu.Lock(); defer m.mu.Unlock(); return m.value }
func (m *userMeter) pending() bool         { m.mu.Lock(); defer m.mu.Unlock(); return m.next != nil }
func (m *userMeter) pendingUsage() (model.Usage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance(m.now())
	if m.next == nil {
		return model.Usage{}, false
	}
	return *m.next, true
}
func (m *userMeter) commitCycle(saved model.Usage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.next == nil {
		return
	}
	if !m.next.CycleStart.Equal(saved.CycleStart) {
		// 提交期间又跨过新边界，保留最新暂存量并基于已提交版本继续推进。
		m.value = saved
		m.next.Version = saved.Version + 1
		m.notify()
		return
	}
	m.value = *m.next
	m.next = nil
	close(m.ready)
	m.ready = nil
	if m.value.Exceeded {
		m.notify()
	}
}
func (s *Service) signalQuota() {
	select {
	case s.quotaEvents <- struct{}{}:
	default:
	}
}

// coordinateQuota 总在服务锁内执行。通知只表示需要重新检查，旧周期通知没有停止新周期的权限。
func (s *Service) coordinateQuota(ctx context.Context) {
	if s.accounts == nil || s.closed {
		return
	}
	for owner, m := range s.meters {
		next, pending := m.pendingUsage()
		if pending {
			if err := s.accounts.ResetUsage(ctx, next); err != nil {
				slog.Error("用户周期推进失败，将重试", "user_id", owner, "error", err)
				continue
			}
			m.commitCycle(next)
			s.persistedUsage[owner] = next
			if m.pending() {
				continue
			}
			// 正常运行的连接沿用同一计量对象；只恢复因配额暂停的规则。
			for id, r := range s.items {
				if r.OwnerID != owner || !r.Enabled || (r.Status != "quota_exceeded" && r.LastErrorCode != "quota_reset_pending") {
					continue
				}
				if m.Snapshot().Exceeded {
					continue
				}
				g, err := s.prepareGroup(r, nil)
				if err != nil {
					s.setError(r, err)
					continue
				}
				r.Status = ""
				r.LastErrorCode = ""
				r.LastError = ""
				s.items[id] = r
				s.startGroup(id, g)
			}
		}
		if !m.Snapshot().Exceeded {
			continue
		}
		for id, r := range s.items {
			if r.OwnerID != owner || !r.Enabled {
				continue
			}
			if g := s.running[id]; g != nil {
				g.stop()
				delete(s.running, id)
			} else if r.Status != "quota_exceeded" {
				continue
			}
			r.Status = "quota_exceeded"
			r.LastError = ""
			r.LastErrorCode = ""
			s.items[id] = r
		}
	}
}
func (s *Service) flushUsages(ctx context.Context) error {
	if s.accounts == nil {
		return nil
	}
	values := make([]model.Usage, 0, len(s.meters))
	for _, m := range s.meters {
		v := m.Snapshot()
		old := s.persistedUsage[v.UserID]
		if v.Version != old.Version || !v.CycleStart.Equal(old.CycleStart) || v.UploadBytes != old.UploadBytes || v.DownloadBytes != old.DownloadBytes {
			values = append(values, v)
		}
	}
	if len(values) == 0 {
		return nil
	}
	if err := s.accounts.SaveUsages(ctx, values); err != nil {
		return err
	}
	for _, v := range values {
		s.persistedUsage[v.UserID] = v
	}
	return nil
}

// settleCycles 在转发停止后完成待提交周期，使正常退出不丢失边界暂存量。
func (s *Service) settleCycles(ctx context.Context) error {
	if s.accounts == nil {
		return nil
	}
	var result error
	for _, m := range s.meters {
		for {
			next, pending := m.pendingUsage()
			if !pending {
				break
			}
			if err := s.accounts.ResetUsage(ctx, next); err != nil {
				result = errors.Join(result, err)
				break
			}
			m.commitCycle(next)
			s.persistedUsage[next.UserID] = next
		}
	}
	return result
}
