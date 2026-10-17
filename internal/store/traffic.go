package store

import (
	"context"
	"fmt"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

// SaveTraffic 由规则服务串行调用，配置更新不会覆盖累计量。
func (s *Store) SaveTraffic(ctx context.Context, values map[string]model.Traffic) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, value := range values {
		if _, err = tx.ExecContext(ctx, `INSERT INTO rule_traffic(rule_id,upload_bytes,download_bytes) VALUES (?,?,?) ON CONFLICT(rule_id) DO UPDATE SET upload_bytes=excluded.upload_bytes,download_bytes=excluded.download_bytes`, id, value.UploadBytes, value.DownloadBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetTraffic 同时提交清零与版本变更，失败时整体回滚。
func (s *Store) ResetTraffic(ctx context.Context, r model.Rule) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE rules SET revision=?,updated_at=? WHERE id=? AND revision=?`, r.Revision, r.UpdatedAt.Format(time.RFC3339Nano), r.ID, r.Revision-1)
	if err != nil {
		return err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return fmt.Errorf("规则版本已改变")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO rule_traffic(rule_id,upload_bytes,download_bytes) VALUES (?,0,0) ON CONFLICT(rule_id) DO UPDATE SET upload_bytes=0,download_bytes=0`, r.ID); err != nil {
		return err
	}
	return tx.Commit()
}
