package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

func TestPersistenceLockAndCorruption(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "porter.db")
	db, e := store.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if other, e := store.Open(path); e == nil {
		other.Close()
		t.Fatal("同一数据库允许了第二个进程")
	}
	now := time.Now().UTC()
	r := model.Rule{ID: "r1", Name: "持久化", Protocol: "udp", ListenHost: "0.0.0.0", ListenPort: 5353, TargetHost: "localhost", TargetPort: 53, Enabled: true, Revision: 1, CreatedAt: now, UpdatedAt: now}
	if e = db.Save(context.Background(), r, true); e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = store.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := db.List(context.Background())
	if e != nil || len(rows) != 1 || rows[0].Name != r.Name || !rows[0].CreatedAt.Equal(now) {
		t.Fatalf("重开数据不一致：%v %v", rows, e)
	}
	db.Close()
	bad := filepath.Join(filepath.Dir(path), "bad.db")
	if e = os.WriteFile(bad, []byte("这不是 SQLite 数据库"), 0600); e != nil {
		t.Fatal(e)
	}
	if db, e = store.Open(bad); e == nil {
		db.Close()
		t.Fatal("损坏数据库未被拒绝")
	}
}
