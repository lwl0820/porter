package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lwl0820/porter/internal/model"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_init.sql
var migration string

type Store struct {
	db   *sql.DB
	lock *os.File
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("数据库已被其他 Porter 实例使用：%w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		lock.Close()
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		lock.Close()
		return nil, err
	}
	s := &Store{db: db, lock: lock}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, stmt := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=DELETE", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON"} {
		if _, err = db.ExecContext(ctx, stmt); err != nil {
			s.Close()
			return nil, err
		}
	}
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		s.Close()
		return nil, err
	}
	if version > 1 {
		s.Close()
		return nil, fmt.Errorf("数据库版本 %d 高于当前程序支持的版本，禁止降级打开", version)
	}
	if version == 0 {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			s.Close()
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, migration); e != nil {
			tx.Rollback()
			s.Close()
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			s.Close()
			return nil, e
		}
	}
	return s, nil
}

func (s *Store) Close() error                   { err := s.db.Close(); s.lock.Close(); return err }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) List(ctx context.Context) ([]model.Rule, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,protocol,listen_host,listen_port,target_host,target_port,enabled,revision,created_at,updated_at FROM rules ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Rule, 0)
	for rows.Next() {
		var r model.Rule
		var created, updated string
		if err = rows.Scan(&r.ID, &r.Name, &r.Protocol, &r.ListenHost, &r.ListenPort, &r.TargetHost, &r.TargetPort, &r.Enabled, &r.Revision, &created, &updated); err != nil {
			return nil, err
		}
		if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, err
		}
		if r.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) Save(ctx context.Context, r model.Rule, create bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := []any{r.Name, r.Protocol, r.ListenHost, r.ListenPort, r.TargetHost, r.TargetPort, r.Enabled, r.Revision, r.CreatedAt.Format(time.RFC3339Nano), r.UpdatedAt.Format(time.RFC3339Nano), r.ID}
	query := "UPDATE rules SET name=?,protocol=?,listen_host=?,listen_port=?,target_host=?,target_port=?,enabled=?,revision=?,created_at=?,updated_at=? WHERE id=?"
	if create {
		query = "INSERT INTO rules (name,protocol,listen_host,listen_port,target_host,target_port,enabled,revision,created_at,updated_at,id) VALUES (?,?,?,?,?,?,?,?,?,?,?)"
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return fmt.Errorf("规则写入数量异常")
	}
	return tx.Commit()
}

func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "DELETE FROM rules WHERE id=?", id)
	if err != nil {
		return err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return fmt.Errorf("规则不存在")
	}
	return tx.Commit()
}
