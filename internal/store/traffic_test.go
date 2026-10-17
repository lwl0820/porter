package store_test

import (
	"context"
	_ "embed"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

//go:embed migrations/002_tcp_udp.sql
var versionTwoMigration string

func TestVersionTwoTrafficMigrationAndPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(testutil.TempDir(t), "v2.db")
	legacy := fixtureDB(t, path)
	execute(t, legacy, versionOneSchema)
	execute(t, legacy, versionTwoMigration)
	execute(t, legacy, `INSERT INTO rules VALUES('legacy','旧规则','tcp+udp','127.0.0.1',12345,'localhost',54321,0,8,'2025-01-02T03:04:05.120000000Z','2025-01-02T03:04:05.120000000Z'); CREATE INDEX custom_rule_idx ON rules(name)`)
	legacy.Close()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].TrafficEnabled || rows[0].Traffic.UploadBytes != 0 || rows[0].Revision != 8 {
		t.Fatalf("旧规则默认值错误：%+v %v", rows, err)
	}
	r := rows[0]
	r.TrafficEnabled = true
	r.Revision++
	if err = db.Save(ctx, r, false); err != nil {
		t.Fatal(err)
	}
	values := map[string]model.Traffic{r.ID: {UploadBytes: math.MaxInt64, DownloadBytes: 9007199254740993}}
	if err = db.SaveTraffic(ctx, values); err != nil {
		t.Fatal(err)
	}
	r.Name = "修改名称"
	if err = db.Save(ctx, r, false); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err = db.List(ctx)
	if err != nil || !rows[0].TrafficEnabled || rows[0].Traffic != values[r.ID] {
		t.Fatalf("统计或开关未恢复：%+v %v", rows, err)
	}
	r = rows[0]
	r.Revision++
	r.UpdatedAt = time.Now().UTC()
	if err = db.ResetTraffic(ctx, r); err != nil {
		t.Fatal(err)
	}
	rows, err = db.List(ctx)
	if err != nil || rows[0].Traffic.UploadBytes != 0 || rows[0].Revision != r.Revision {
		t.Fatalf("重置未原子保存：%+v %v", rows, err)
	}
	if err = db.ResetTraffic(ctx, r); err == nil {
		t.Fatal("过期版本可重置")
	}
	if err = db.SaveTraffic(ctx, values); err != nil {
		t.Fatal(err)
	}
	if err = db.Delete(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	inspect := fixtureDB(t, path)
	assertVersion(t, inspect, 3)
	var count int
	if err = inspect.QueryRow("SELECT count(*) FROM rule_traffic").Scan(&count); err != nil || count != 0 {
		t.Fatalf("删除未清理统计：%d %v", count, err)
	}
	if err = db.SaveTraffic(ctx, values); err == nil {
		t.Fatal("统计写入复活已删除规则")
	}
}

func TestTrafficMigrationAndResetFailuresRollback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(testutil.TempDir(t), "rollback.db")
	legacy := fixtureDB(t, path)
	execute(t, legacy, versionOneSchema)
	execute(t, legacy, versionTwoMigration)
	execute(t, legacy, "CREATE TABLE rule_traffic(custom TEXT)")
	legacy.Close()
	if db, err := store.Open(path); err == nil {
		db.Close()
		t.Fatal("冲突表未阻止迁移")
	}
	inspect := fixtureDB(t, path)
	assertVersion(t, inspect, 2)
	var columns int
	if err := inspect.QueryRow("SELECT count(*) FROM pragma_table_info('rules') WHERE name='traffic_enabled'").Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("失败迁移未回滚新增字段：%d %v", columns, err)
	}
	execute(t, inspect, "DROP TABLE rule_traffic")
	inspect.Close()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	r := model.Rule{ID: "r", Name: "规则", Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: 12345, TargetHost: "localhost", TargetPort: 54321, Revision: 1, CreatedAt: now, UpdatedAt: now}
	if err = db.Save(ctx, r, true); err != nil {
		t.Fatal(err)
	}
	values := map[string]model.Traffic{r.ID: {UploadBytes: 100, DownloadBytes: 200}}
	if err = db.SaveTraffic(ctx, values); err != nil {
		t.Fatal(err)
	}
	inspect = fixtureDB(t, path)
	execute(t, inspect, "CREATE TRIGGER fail_traffic BEFORE UPDATE ON rule_traffic BEGIN SELECT RAISE(FAIL, '注入统计失败'); END")
	r.Revision++
	if err = db.ResetTraffic(ctx, r); err == nil {
		t.Fatal("重置失败未返回")
	}
	rows, err := db.List(ctx)
	if err != nil || rows[0].Revision != 1 || rows[0].Traffic != values[r.ID] {
		t.Fatalf("重置失败部分提交：%+v %v", rows, err)
	}
}
