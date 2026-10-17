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
var initialMigration string

//go:embed migrations/002_tcp_udp.sql
var combinedMigration string

//go:embed migrations/003_traffic.sql
var trafficMigration string

//go:embed migrations/004_accounts.sql
var accountsMigration string

var migrations = []string{initialMigration, combinedMigration, trafficMigration, accountsMigration}

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
	if err = migrate(ctx, db); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) || version < 0 {
		return fmt.Errorf("数据库版本 %d 不在当前程序支持的范围内，禁止降级打开", version)
	}
	for version < len(migrations) {
		if err := applyMigration(ctx, db, version+1); err != nil {
			return fmt.Errorf("数据库迁移到版本 %d 失败：%w", version+1, err)
		}
		version++
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var objects []string
	if version == 2 {
		objects, err = tableObjects(ctx, tx)
		if err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, migrations[version-1]); err != nil {
		return err
	}
	for _, statement := range objects {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("恢复 rules 附属对象失败：%w", err)
		}
	}
	return tx.Commit()
}

// tableObjects 在删除旧表前保存附属对象；遇到无法无损处理的自定义结构则拒绝迁移。
func tableObjects(ctx context.Context, tx *sql.Tx) ([]string, error) {
	columns, err := tx.QueryContext(ctx, "SELECT name FROM pragma_table_xinfo('rules') ORDER BY cid")
	if err != nil {
		return nil, err
	}
	expected := []string{"id", "name", "protocol", "listen_host", "listen_port", "target_host", "target_port", "enabled", "revision", "created_at", "updated_at"}
	var names []string
	for columns.Next() {
		var name string
		if err = columns.Scan(&name); err != nil {
			columns.Close()
			return nil, err
		}
		names = append(names, name)
	}
	err = columns.Err()
	columns.Close()
	if err != nil {
		return nil, err
	}
	if len(names) != len(expected) {
		return nil, fmt.Errorf("rules 表字段与版本 1 不一致，无法安全迁移")
	}
	for i, name := range names {
		if name != expected[i] {
			return nil, fmt.Errorf("rules 表字段与版本 1 不一致，无法安全迁移")
		}
	}
	var references int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema AS s JOIN pragma_foreign_key_list(s.name) AS f WHERE s.type='table' AND (lower(f."table")='rules' OR lower(s.name)='rules')`).Scan(&references); err != nil {
		return nil, err
	}
	if references != 0 {
		return nil, fmt.Errorf("存在涉及 rules 的外键，无法安全重建规则表")
	}
	rows, err := tx.QueryContext(ctx, "SELECT sql FROM sqlite_schema WHERE tbl_name='rules' AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var objects []string
	for rows.Next() {
		var statement string
		if err = rows.Scan(&statement); err != nil {
			return nil, err
		}
		objects = append(objects, statement)
	}
	return objects, rows.Err()
}

func (s *Store) Close() error                   { err := s.db.Close(); s.lock.Close(); return err }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) List(ctx context.Context) ([]model.Rule, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT r.id,r.name,r.protocol,r.listen_host,r.listen_port,r.target_host,r.target_port,r.enabled,r.revision,r.created_at,r.updated_at,r.traffic_enabled,COALESCE(t.upload_bytes,0),COALESCE(t.download_bytes,0),r.owner_id FROM rules r LEFT JOIN rule_traffic t ON t.rule_id=r.id ORDER BY r.created_at,r.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Rule, 0)
	for rows.Next() {
		var r model.Rule
		var created, updated string
		if err = rows.Scan(&r.ID, &r.Name, &r.Protocol, &r.ListenHost, &r.ListenPort, &r.TargetHost, &r.TargetPort, &r.Enabled, &r.Revision, &created, &updated, &r.TrafficEnabled, &r.Traffic.UploadBytes, &r.Traffic.DownloadBytes, &r.OwnerID); err != nil {
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
	if r.OwnerID == "" {
		r.OwnerID = model.AdminID
	}
	args := []any{r.OwnerID, r.Name, r.Protocol, r.ListenHost, r.ListenPort, r.TargetHost, r.TargetPort, r.Enabled, r.TrafficEnabled, r.Revision, r.CreatedAt.Format(time.RFC3339Nano), r.UpdatedAt.Format(time.RFC3339Nano), r.ID}
	query := "UPDATE rules SET owner_id=?,name=?,protocol=?,listen_host=?,listen_port=?,target_host=?,target_port=?,enabled=?,traffic_enabled=?,revision=?,created_at=?,updated_at=? WHERE id=?"
	if create {
		query = "INSERT INTO rules (owner_id,name,protocol,listen_host,listen_port,target_host,target_port,enabled,traffic_enabled,revision,created_at,updated_at,id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)"
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
