package store_test

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

//go:embed migrations/003_traffic.sql
var versionThreeMigration string

func accountStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(testutil.TempDir(t), "accounts.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.EnsureAdmin(context.Background(), "管理员"); err != nil {
		t.Fatal(err)
	}
	return db, path
}

func testPlan(t *testing.T, db *store.Store, id string, count int) model.Plan {
	t.Helper()
	p := model.Plan{ID: id, Name: "套餐" + id, PortCount: count, TrafficLimitBytes: 1000, AccountingMode: model.AccountingMax, ResetMode: model.ResetAnniversary, CreatedAt: time.Date(2025, 1, 31, 12, 0, 0, 0, time.UTC)}
	if err := db.CreatePlan(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func testUser(t *testing.T, db *store.Store, id, planID string, managementPort int) model.User {
	t.Helper()
	u, err := db.CreateUser(context.Background(), model.User{ID: id, Username: id, PlanID: &planID, PasswordHash: "测试派生编码", CreatedAt: time.Date(2025, 1, 31, 12, 0, 0, 0, time.UTC)}, model.Usage{}, managementPort)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func settings(t *testing.T, db *store.Store, start, end int) {
	t.Helper()
	v, err := db.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v.PortStart, v.PortEnd = start, end
	v.Revision++
	if err = db.SaveSettings(context.Background(), v); err != nil {
		t.Fatal(err)
	}
}

func TestAccountMigrationV3PreservesUnknownColumnsObjectsAndTraffic(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(testutil.TempDir(t), "v3.db")
	legacy := fixtureDB(t, path)
	execute(t, legacy, versionOneSchema)
	execute(t, legacy, versionTwoMigration)
	execute(t, legacy, versionThreeMigration)
	execute(t, legacy, `ALTER TABLE rules ADD COLUMN custom TEXT DEFAULT '原值';
	CREATE TABLE audit(id TEXT);
	CREATE INDEX original_index ON rules(name);
	CREATE TRIGGER original_trigger AFTER UPDATE OF name ON rules BEGIN INSERT INTO audit VALUES(new.id); END;
	INSERT INTO rules(id,name,protocol,listen_host,listen_port,target_host,target_port,enabled,revision,created_at,updated_at,traffic_enabled,custom) VALUES('legacy','原规则','tcp+udp','127.0.0.1',12000,'localhost',13000,1,17,'2025-01-01T12:34:56.123456789Z','2025-02-02T12:34:56.120000000Z',1,'自定义');
	INSERT INTO rule_traffic VALUES('legacy',9007199254740993,123)`)
	legacy.Close()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	admin, err := db.EnsureAdmin(ctx, "原管理员")
	if err != nil {
		t.Fatal(err)
	}
	if admin.ID != model.AdminID || admin.PasswordHash != "" || admin.PlanID != nil {
		t.Fatal("环境管理员错误")
	}
	rows, err := db.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].OwnerID != model.AdminID || rows[0].Revision != 17 || rows[0].Traffic.UploadBytes != 9007199254740993 || rows[0].Protocol != model.ProtocolBoth {
		t.Fatalf("旧库升级丢失字段：%+v %v", rows, err)
	}
	inspect := fixtureDB(t, path)
	assertVersion(t, inspect, 4)
	var custom, raw string
	if err = inspect.QueryRow("SELECT custom,updated_at FROM rules WHERE id='legacy'").Scan(&custom, &raw); err != nil || custom != "自定义" || raw != "2025-02-02T12:34:56.120000000Z" {
		t.Fatalf("增量迁移修改旧列：%s %s %v", custom, raw, err)
	}
	rows[0].Name = "更新"
	if err = db.Save(ctx, rows[0], false); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = inspect.QueryRow("SELECT count(*) FROM audit WHERE id='legacy'").Scan(&count); err != nil || count != 1 {
		t.Fatal("原触发器失效")
	}
	if err = inspect.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='original_index'").Scan(&count); err != nil || count != 1 {
		t.Fatal("原索引丢失")
	}
}

func TestAccountMigrationFailureRollsBackAllNewObjects(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "v3-failure.db")
	legacy := fixtureDB(t, path)
	execute(t, legacy, versionOneSchema)
	execute(t, legacy, versionTwoMigration)
	execute(t, legacy, versionThreeMigration)
	execute(t, legacy, "CREATE TABLE user_ports(original TEXT); INSERT INTO user_ports VALUES('保留')")
	legacy.Close()
	if db, err := store.Open(path); err == nil {
		db.Close()
		t.Fatal("命名冲突迁移未失败")
	}
	inspect := fixtureDB(t, path)
	assertVersion(t, inspect, 3)
	var count int
	if err := inspect.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name IN ('plans','users','system_settings')").Scan(&count); err != nil || count != 0 {
		t.Fatal("失败迁移遗留新表")
	}
	if err := inspect.QueryRow("SELECT count(*) FROM pragma_table_info('rules') WHERE name='owner_id'").Scan(&count); err != nil || count != 0 {
		t.Fatal("失败迁移遗留owner列")
	}
	var original string
	if err := inspect.QueryRow("SELECT original FROM user_ports").Scan(&original); err != nil || original != "保留" {
		t.Fatal("失败迁移破坏已有自定义表")
	}
}

func TestPortAllocationAtomicityExclusionsAndPoolUpdates(t *testing.T) {
	ctx := context.Background()
	db, path := accountStore(t)
	p := testPlan(t, db, "p", 3)
	candidate := model.User{ID: "未分配", Username: "未分配", PasswordHash: "派生结果", PlanID: &p.ID, CreatedAt: p.CreatedAt}
	if _, err := db.CreateUser(ctx, candidate, model.Usage{}, 10007); !errors.Is(err, store.ErrPortPoolUnset) {
		t.Fatalf("未配置仍分配：%v", err)
	}
	settings(t, db, 10000, 10011)
	rule := model.Rule{ID: "admin-rule", Name: "已停用管理员规则", Protocol: model.ProtocolTCP, ListenHost: "127.0.0.1", ListenPort: 10001, TargetHost: "localhost", TargetPort: 20000, Revision: 1, CreatedAt: p.CreatedAt, UpdatedAt: p.CreatedAt}
	if err := db.Save(ctx, rule, true); err != nil {
		t.Fatal(err)
	}
	u1 := testUser(t, db, "u1", p.ID, 10007)
	u2 := testUser(t, db, "u2", p.ID, 10007)
	if u1.PortStart != 10002 || u1.PortEnd != 10004 || u2.PortStart != 10008 || u2.PortEnd != 10010 {
		t.Fatalf("端口未排除管理监听及管理员停用规则：%+v %+v", u1, u2)
	}
	if _, err := db.CreateUser(ctx, candidate, model.Usage{}, 10007); !errors.Is(err, store.ErrPortExhausted) {
		t.Fatalf("碎片不足未失败：%v", err)
	}
	users, err := db.ListUsers(ctx)
	if err != nil || len(users) != 3 {
		t.Fatalf("失败分配留下账号：%+v %v", users, err)
	}
	inspect := fixtureDB(t, path)
	var ports int
	if err = inspect.QueryRow("SELECT count(*) FROM user_ports").Scan(&ports); err != nil || ports != 6 {
		t.Fatal("失败分配残留端口")
	}
	settings(t, db, 10004, 10014)
	u3 := testUser(t, db, "u3", p.ID, 10007)
	if u3.PortStart != 10011 || u3.PortEnd != 10013 {
		t.Fatalf("修改池后侵占旧范围或不在新范围：%+v", u3)
	}
	read, err := db.GetUser(ctx, u1.ID)
	if err != nil || read.PortStart != u1.PortStart || read.PortEnd != u1.PortEnd {
		t.Fatal("修改池迁移已有用户")
	}
	settings(t, db, 10100, 10110)
	candidate.Username = "u1"
	if _, err = db.CreateUser(ctx, candidate, model.Usage{}, 10007); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("重名未拒绝：%v", err)
	}
	u4 := testUser(t, db, "u4", p.ID, 10007)
	if u4.PortStart != 10100 {
		t.Fatal("重名失败消耗了端口")
	}
}

func TestConcurrentUserAllocationNeverOverlapsAndInitializesCycles(t *testing.T) {
	ctx := context.Background()
	db, _ := accountStore(t)
	p := testPlan(t, db, "p", 2)
	settings(t, db, 20000, 20039)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("并发%d", i)
			_, err := db.CreateUser(ctx, model.User{ID: id, Username: id, PasswordHash: "派生结果", PlanID: &p.ID, CreatedAt: p.CreatedAt}, model.Usage{}, 8080)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	users, err := db.ListUsers(ctx)
	if err != nil || len(users) != 21 {
		t.Fatalf("并发账号：%d %v", len(users), err)
	}
	seen := map[int]bool{}
	for _, u := range users {
		if u.Role == model.RoleAdmin {
			continue
		}
		if u.PortEnd-u.PortStart+1 != 2 {
			t.Fatal("范围长度不等于套餐")
		}
		for port := u.PortStart; port <= u.PortEnd; port++ {
			if seen[port] {
				t.Fatal("并发分配重叠")
			}
			seen[port] = true
		}
	}
	usages, err := db.ListUsages(ctx)
	if err != nil || len(usages) != 20 {
		t.Fatal("创建用户未同时创建账本")
	}
	for _, u := range usages {
		if !u.CycleStart.Equal(p.CreatedAt) || !u.CycleEnd.Equal(time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC)) || u.Version != 1 || u.UploadBytes != 0 || u.LimitBytes != 1000 {
			t.Fatalf("首周期错误：%+v", u)
		}
	}
	if len(seen) != 40 {
		t.Fatal("分配端口总数错误")
	}
}

func TestFailedUserUsageInsertDoesNotLeaveUserOrPorts(t *testing.T) {
	ctx := context.Background()
	db, path := accountStore(t)
	p := testPlan(t, db, "p", 3)
	settings(t, db, 30000, 30010)
	inspect := fixtureDB(t, path)
	execute(t, inspect, "CREATE TRIGGER fail_usage BEFORE INSERT ON user_usage BEGIN SELECT RAISE(FAIL,'注入初始周期失败'); END")
	if _, err := db.CreateUser(ctx, model.User{ID: "失败", Username: "失败", PasswordHash: "派生结果", PlanID: &p.ID, CreatedAt: p.CreatedAt}, model.Usage{}, 8080); err == nil {
		t.Fatal("初始周期故障未返回")
	}
	users, err := db.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatal("事务失败留下账号")
	}
	var count int
	if err = inspect.QueryRow("SELECT count(*) FROM user_ports").Scan(&count); err != nil || count != 0 {
		t.Fatal("事务失败留下端口")
	}
	execute(t, inspect, "DROP TRIGGER fail_usage")
	u := testUser(t, db, "成功", p.ID, 8080)
	if u.PortStart != 30000 {
		t.Fatal("回滚后端口未重新可用")
	}
}

func TestAdminRenameKeepsIdentityAndRejectsUserConflict(t *testing.T) {
	ctx := context.Background()
	db, _ := accountStore(t)
	p := testPlan(t, db, "p", 1)
	settings(t, db, 31000, 31010)
	u := testUser(t, db, "普通用户", p.ID, 8080)
	original, err := db.GetUser(ctx, model.AdminID)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := db.EnsureAdmin(ctx, "新管理员")
	if err != nil || renamed.ID != original.ID || !renamed.CreatedAt.Equal(original.CreatedAt) {
		t.Fatal("管理员改名改变身份")
	}
	if _, err = db.EnsureAdmin(ctx, u.Username); !errors.Is(err, store.ErrConflict) {
		t.Fatal("管理员改名接管普通用户")
	}
	got, err := db.GetUser(ctx, model.AdminID)
	if err != nil || got.Username != "新管理员" {
		t.Fatal("失败改名未回滚")
	}
	got, err = db.UserByUsername(ctx, u.Username)
	if err != nil || got.ID != u.ID || got.Role != model.RoleUser {
		t.Fatal("管理员冲突损坏普通用户身份")
	}
}

func TestUsagePersistenceOldSnapshotsAndCycleTransactions(t *testing.T) {
	ctx := context.Background()
	db, path := accountStore(t)
	p := testPlan(t, db, "p", 1)
	settings(t, db, 32000, 32010)
	testUser(t, db, "用户", p.ID, 8080)
	values, err := db.ListUsages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old := values[0]
	old.UploadBytes = math.MaxInt64
	old.DownloadBytes = 9007199254740993
	if err = db.SaveUsages(ctx, []model.Usage{old}); err != nil {
		t.Fatal(err)
	}
	values, err = db.ListUsages(ctx)
	if err != nil || values[0].UploadBytes != old.UploadBytes || values[0].UsedBytes != math.MaxInt64 || !values[0].Exceeded {
		t.Fatal("精确用量未保存或计算错误")
	}
	stale := old
	stale.UploadBytes = 1
	stale.DownloadBytes = 1
	if err = db.SaveUsages(ctx, []model.Usage{stale}); err != nil {
		t.Fatal(err)
	}
	values, err = db.ListUsages(ctx)
	if err != nil || values[0].UploadBytes != old.UploadBytes {
		t.Fatal("同周期旧快照返还额度")
	}
	next := old
	next.Version++
	next.CycleStart = old.CycleEnd
	next.CycleEnd = time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	next.UploadBytes = 7
	next.DownloadBytes = 8
	inspect := fixtureDB(t, path)
	execute(t, inspect, "CREATE TRIGGER fail_cycle BEFORE UPDATE ON user_usage BEGIN SELECT RAISE(FAIL,'注入周期失败'); END")
	if err = db.ResetUsage(ctx, next); err == nil {
		t.Fatal("周期提交失败未返回")
	}
	values, err = db.ListUsages(ctx)
	if err != nil || values[0].Version != old.Version || values[0].UploadBytes != old.UploadBytes {
		t.Fatal("失败周期提前发放额度")
	}
	execute(t, inspect, "DROP TRIGGER fail_cycle")
	if err = db.ResetUsage(ctx, next); err != nil {
		t.Fatal(err)
	}
	if err = db.ResetUsage(ctx, next); !errors.Is(err, store.ErrRevision) {
		t.Fatal("重复重置返还额度")
	}
	if err = db.SaveUsages(ctx, []model.Usage{old}); !errors.Is(err, store.ErrRevision) {
		t.Fatal("旧周期覆盖新周期")
	}
	values, err = db.ListUsages(ctx)
	if err != nil || values[0].UploadBytes != 7 || values[0].DownloadBytes != 8 || values[0].Version != 2 {
		t.Fatal("旧快照破坏新周期增量")
	}
	backwards := old
	backwards.Version = 3
	if err = db.ResetUsage(ctx, backwards); !errors.Is(err, store.ErrRevision) {
		t.Fatal("时钟回退重新发放额度")
	}
	batch := values[0]
	batch.UploadBytes = 55
	missing := batch
	missing.UserID = "不存在"
	if err = db.SaveUsages(ctx, []model.Usage{batch, missing}); err == nil {
		t.Fatal("批量错误未返回")
	}
	values, err = db.ListUsages(ctx)
	if err != nil || values[0].UploadBytes != 7 {
		t.Fatal("批量保存部分提交")
	}
}

func TestSettingsRevisionAndPlanDatabaseConstraints(t *testing.T) {
	ctx := context.Background()
	db, _ := accountStore(t)
	before, err := db.Settings(ctx)
	if err != nil || before.PortStart != 0 || before.PortEnd != 0 || before.Revision != 1 {
		t.Fatal("未配置设置初值错误")
	}
	v := model.Settings{PortStart: 1, PortEnd: 65535, Revision: 2}
	if err = db.SaveSettings(ctx, v); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveSettings(ctx, v); !errors.Is(err, store.ErrRevision) {
		t.Fatal("设置过期版本未拒绝")
	}
	v.Revision++
	v.PortEnd = 65536
	if err = db.SaveSettings(ctx, v); err == nil {
		t.Fatal("设置非法端口被接受")
	}
	good := model.Plan{ID: "good", Name: "套餐", PortCount: 1, TrafficLimitBytes: 1, AccountingMode: model.AccountingBoth, ResetMode: model.ResetCalendarMonth, CreatedAt: time.Now().UTC()}
	for i, mutate := range []func(*model.Plan){
		func(p *model.Plan) { p.PortCount = 0 }, func(p *model.Plan) { p.PortCount = 65536 }, func(p *model.Plan) { p.TrafficLimitBytes = 0 }, func(p *model.Plan) { p.AccountingMode = "bad" }, func(p *model.Plan) { p.ResetMode = "bad" }, func(p *model.Plan) { p.ResetMode = model.ResetFixedDay }, func(p *model.Plan) { d := 31; p.ResetDay = &d }, func(p *model.Plan) { d := 32; p.ResetDay = &d; p.ResetMode = model.ResetFixedDay },
	} {
		p := good
		p.ID = fmt.Sprintf("bad%d", i)
		mutate(&p)
		if err = db.CreatePlan(ctx, p); err == nil {
			t.Fatalf("非法套餐%d未拒绝", i)
		}
	}
}

func TestFirstCycleOneNanosecondBeforeReset(t *testing.T) {
	ctx := context.Background()
	db, _ := accountStore(t)
	p := testPlan(t, db, "p", 1)
	p.ResetMode = model.ResetCalendarMonth
	p.ID = "calendar"
	if err := db.CreatePlan(ctx, p); err != nil {
		t.Fatal(err)
	}
	settings(t, db, 33000, 33010)
	created := time.Date(2025, 1, 31, 23, 59, 59, 999999999, time.UTC)
	_, err := db.CreateUser(ctx, model.User{ID: "临界开户", Username: "临界开户", PlanID: &p.ID, PasswordHash: "派生结果", CreatedAt: created}, model.Usage{}, 8080)
	if err != nil {
		t.Fatal(err)
	}
	values, err := db.ListUsages(ctx)
	if err != nil || len(values) != 1 {
		t.Fatal(err)
	}
	next := values[0]
	next.Version++
	next.CycleStart = next.CycleEnd
	next.CycleEnd = time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	if err = db.ResetUsage(ctx, next); err != nil {
		t.Fatalf("纳秒相邻周期被精度丢失误判：%v", err)
	}
}
