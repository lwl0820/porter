package rules

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

func (s *Service) trafficLoop() {
	defer close(s.trafficDone)
	sample := time.NewTicker(time.Second)
	flush := time.NewTicker(s.flushInterval)
	defer sample.Stop()
	defer flush.Stop()
	for {
		select {
		case <-s.stopTraffic:
			return
		case <-s.quotaEvents:
			s.mu.Lock()
			s.coordinateQuota(context.Background())
			s.mu.Unlock()
		case now := <-sample.C:
			s.mu.Lock()
			s.coordinateQuota(context.Background())
			for id, t := range s.traffic {
				t.Sample(now, s.running[id] != nil && s.items[id].Enabled)
			}
			s.mu.Unlock()
		case <-flush.C:
			s.mu.Lock()
			err := errors.Join(s.flushTraffic(context.Background()), s.flushUsages(context.Background()))
			s.mu.Unlock()
			if err != nil {
				slog.Error("流量统计保存失败，将在下次刷新重试", "error", err)
			}
		}
	}
}

// flushTraffic 的调用方持有服务锁；只在提交成功后推进持久化基线。
func (s *Service) flushTraffic(ctx context.Context) error {
	values := make(map[string]model.Traffic)
	for id, t := range s.traffic {
		v := t.Snapshot()
		old := s.persisted[id]
		if v.UploadBytes != old.UploadBytes || v.DownloadBytes != old.DownloadBytes {
			values[id] = v
		}
	}
	if len(values) == 0 {
		return nil
	}
	if err := s.repo.SaveTraffic(ctx, values); err != nil {
		return err
	}
	for id, v := range values {
		s.persisted[id] = v
	}
	return nil
}

func (s *Service) ResetTraffic(ctx context.Context, id string, revision int64) (model.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return model.Rule{}, problem(503, "shutting_down", "服务正在停止")
	}
	return s.resetTrafficLocked(ctx, id, revision)
}

func (s *Service) resetTrafficLocked(ctx context.Context, id string, revision int64) (model.Rule, error) {
	r, ok := s.items[id]
	if !ok {
		return model.Rule{}, problem(404, "not_found", "规则不存在")
	}
	if r.Revision != revision {
		return model.Rule{}, problem(412, "revision_mismatch", "规则已被修改，请刷新后重试")
	}
	t := s.traffic[id]
	old := t.BeginReset(time.Now())
	r.Revision++
	r.UpdatedAt = time.Now().UTC()
	if err := s.repo.ResetTraffic(ctx, r); err != nil {
		t.RollbackReset(old)
		slog.Error("重置流量失败", "rule_id", id, "error", err)
		return model.Rule{}, problem(500, "database_error", "重置失败，原统计保持不变")
	}
	s.persisted[id] = model.Traffic{}
	s.items[id] = r
	return s.view(r), nil
}
