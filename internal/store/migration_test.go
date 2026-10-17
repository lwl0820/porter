package store_test

import (
	"context"
	"database/sql"
	_ "embed"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

//go:embed migrations/001_init.sql
var versionOneSchema string

func fixtureDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func execute(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatal(err)
	}
}

func assertVersion(t *testing.T, db *sql.DB, expected int) {
	t.Helper()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != expected {
		t.Fatalf("数据库版本为 %d，期望 %d", version, expected)
	}
}

func TestVersionOneMigrationPreservesRecordsAndObjects(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "v1.db")
	legacy := fixtureDB(t, path)
	execute(t, legacy, versionOneSchema)
	created := "2025-01-02T03:04:05.123456789Z"
	updated := "2025-06-07T08:09:10.120000000Z"
	for i, protocol := range []string{"tcp", "udp"} {
		execute(t, legacy, `INSERT INTO rules (id,name,protocol,listen_host,listen_port,target_host,target_port,enabled,revision,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			"old-"+protocol, "旧规则 "+protocol, protocol, "0.0.0.0", 12345+i, "example.com", 4567+i, i, 7+i, created, updated)
	}
	execute(t, legacy, `CREATE TABLE audit (rule_id TEXT, name TEXT);
		CREATE INDEX rules_name_idx ON rules(name);
		CREATE TRIGGER rules_name_audit AFTER UPDATE OF name ON rules BEGIN INSERT INTO audit(rule_id,name) VALUES(new.id,new.name); END;`)
	legacy.Close()

	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.List(context.Background())
	if err != nil || len(rows) != 2 {
		t.Fatalf("旧规则数量或读取失败：%v %v", rows, err)
	}
	createdTime, _ := time.Parse(time.RFC3339Nano, created)
	updatedTime, _ := time.Parse(time.RFC3339Nano, updated)
	for i, row := range rows {
		protocol := []string{"tcp", "udp"}[i]
		expected := model.Rule{ID: "old-" + protocol, Name: "旧规则 " + protocol, Protocol: protocol, ListenHost: "0.0.0.0", ListenPort: 12345 + i, TargetHost: "example.com", TargetPort: 4567 + i, Enabled: i == 1, Revision: int64(7 + i), CreatedAt: createdTime, UpdatedAt: updatedTime}
		if !reflect.DeepEqual(row, expected) {
			t.Fatalf("旧规则迁移后变化：实际 %#v，期望 %#v", row, expected)
		}
	}
	inspection := fixtureDB(t, path)
	assertVersion(t, inspection, 3)
	var rawCreated, rawUpdated string
	if err = inspection.QueryRow("SELECT created_at,updated_at FROM rules WHERE id='old-tcp'").Scan(&rawCreated, &rawUpdated); err != nil || rawCreated != created || rawUpdated != updated {
		t.Fatalf("原始时间文本变化：%q %q %v", rawCreated, rawUpdated, err)
	}
	var objects int
	if err = inspection.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name IN ('rules_name_idx','rules_name_audit')").Scan(&objects); err != nil || objects != 2 {
		t.Fatalf("附属索引或触发器丢失：%d %v", objects, err)
	}
	rows[0].Name = "修改后"
	rows[0].Revision++
	if err = db.Save(context.Background(), rows[0], false); err != nil {
		t.Fatal(err)
	}
	var audited string
	if err = inspection.QueryRow("SELECT name FROM audit WHERE rule_id='old-tcp'").Scan(&audited); err != nil || audited != "修改后" {
		t.Fatalf("迁移后触发器未生效：%q %v", audited, err)
	}
}

func TestFreshAndVersionTwoDatabaseSupportCombinedProtocol(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "fresh.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rule := model.Rule{ID: "both", Name: "组合规则", Protocol: model.ProtocolBoth, ListenHost: "::1", ListenPort: 12345, TargetHost: "localhost", TargetPort: 54321, Enabled: true, Revision: 4, CreatedAt: now, UpdatedAt: now}
	if err = db.Save(context.Background(), rule, true); err != nil {
		t.Fatal(err)
	}
	db.Close()
	inspection := fixtureDB(t, path)
	assertVersion(t, inspection, 3)
	execute(t, inspection, "CREATE TABLE version_two_marker(value TEXT); INSERT INTO version_two_marker VALUES('保留')")
	inspection.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.List(context.Background())
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], rule) {
		t.Fatalf("组合规则重开不一致：%#v %v", rows, err)
	}
	inspection = fixtureDB(t, path)
	var marker string
	if err = inspection.QueryRow("SELECT value FROM version_two_marker").Scan(&marker); err != nil || marker != "保留" {
		t.Fatalf("版本 2 重开不应重建已有结构：%q %v", marker, err)
	}
	for _, statement := range []string{
		"INSERT INTO rules SELECT 'duplicate',name,protocol,listen_host,listen_port,target_host,target_port,enabled,revision,created_at,updated_at,traffic_enabled FROM rules WHERE id='both'",
		"UPDATE rules SET protocol='invalid' WHERE id='both'",
		"UPDATE rules SET listen_port=0 WHERE id='both'",
		"UPDATE rules SET target_port=65536 WHERE id='both'",
		"UPDATE rules SET enabled=2 WHERE id='both'",
		"UPDATE rules SET revision=0 WHERE id='both'",
		"UPDATE rules SET name=NULL WHERE id='both'",
	} {
		if _, err = inspection.Exec(statement); err == nil {
			t.Fatalf("迁移后约束未保留：%s", statement)
		}
	}
}

func TestFailedMigrationRollsBackSchemaDataAndVersion(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "invalid-v1.db")
	legacy := fixtureDB(t, path)
	execute(t, legacy, versionOneSchema)
	// 构造真实旧库异常记录，使新表已经创建后在复制阶段触发 CHECK 失败。
	execute(t, legacy, "PRAGMA ignore_check_constraints=ON")
	execute(t, legacy, `INSERT INTO rules VALUES('invalid','旧异常规则','invalid','127.0.0.1',12345,'localhost',12345,1,9,'2025-01-01T00:00:00Z','2025-01-02T00:00:00Z')`)
	execute(t, legacy, "CREATE INDEX rules_name_idx ON rules(name)")
	legacy.Close()
	if db, err := store.Open(path); err == nil {
		db.Close()
		t.Fatal("异常旧数据应导致迁移失败")
	}
	inspection := fixtureDB(t, path)
	assertVersion(t, inspection, 1)
	var protocol string
	if err := inspection.QueryRow("SELECT protocol FROM rules WHERE id='invalid'").Scan(&protocol); err != nil || protocol != "invalid" {
		t.Fatalf("失败迁移改变了旧数据：%q %v", protocol, err)
	}
	var temporary, indexes int
	if err := inspection.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='rules_v2'").Scan(&temporary); err != nil || temporary != 0 {
		t.Fatalf("失败迁移遗留半迁移表：%d %v", temporary, err)
	}
	if err := inspection.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='rules_name_idx'").Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("失败迁移丢失旧索引：%d %v", indexes, err)
	}
	execute(t, inspection, "UPDATE rules SET protocol='tcp' WHERE id='invalid'")
	inspection.Close()
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("修复旧库后应可重新迁移：%v", err)
	}
	db.Close()
}

func TestUnsafeCustomSchemaAndFutureVersionsAreRejected(t *testing.T) {
	for _, test := range []struct{ name, statement, expected string }{
		{"新增列", "ALTER TABLE rules ADD COLUMN custom_value TEXT", "字段"},
		{"生成列", "ALTER TABLE rules ADD COLUMN custom_value TEXT GENERATED ALWAYS AS (name) VIRTUAL", "字段"},
		{"外部外键", "CREATE TABLE reference_rule(rule_id TEXT REFERENCES rules(id))", "外键"},
		{"未来版本", "PRAGMA user_version=4", "版本"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(testutil.TempDir(t), "custom.db")
			legacy := fixtureDB(t, path)
			execute(t, legacy, versionOneSchema)
			execute(t, legacy, test.statement)
			legacy.Close()
			db, err := store.Open(path)
			if err == nil {
				db.Close()
				t.Fatal("不支持的数据库结构未被拒绝")
			}
			if !strings.Contains(err.Error(), test.expected) {
				t.Fatalf("错误原因不明确：%v", err)
			}
			inspection := fixtureDB(t, path)
			expectedVersion := 1
			if test.name == "未来版本" {
				expectedVersion = 4
			}
			assertVersion(t, inspection, expectedVersion)
		})
	}
}
