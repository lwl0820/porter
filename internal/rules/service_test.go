package rules_test

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

type faultRepo struct {
	*store.Store
	fail bool
}

func (r *faultRepo) Save(ctx context.Context, rule model.Rule, create bool) error {
	if r.fail {
		return errors.New("注入数据库提交失败")
	}
	return r.Store.Save(ctx, rule, create)
}
func (r *faultRepo) Delete(ctx context.Context, id string) error {
	if r.fail {
		return errors.New("注入删除失败")
	}
	return r.Store.Delete(ctx, id)
}
func input(port, target int) model.Input {
	enabled := true
	return model.Input{Name: "测试规则", Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: port, TargetHost: "127.0.0.1", TargetPort: target, Enabled: &enabled}
}
func freePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func echo(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}
func exchange(t *testing.T, c net.Conn, text string) {
	t.Helper()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, e := c.Write([]byte(text)); e != nil {
		t.Fatal(e)
	}
	buf := make([]byte, len(text))
	if _, e := io.ReadFull(c, buf); e != nil {
		t.Fatal(e)
	}
	if string(buf) != text {
		t.Fatalf("响应不匹配：%q", buf)
	}
}
func setup(t *testing.T) (*rules.Service, *faultRepo) {
	t.Helper()
	db, e := store.Open(filepath.Join(testutil.TempDir(t), "rules.db"))
	if e != nil {
		t.Fatal(e)
	}
	repo := &faultRepo{Store: db}
	s, e := rules.New(context.Background(), repo, forward.New(forward.Limits{}), "127.0.0.1:8080")
	if e != nil {
		db.Close()
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close(); db.Close() })
	return s, repo
}
func status(t *testing.T, err error, want int) {
	t.Helper()
	var e *rules.Error
	if !errors.As(err, &e) || e.Status != want {
		t.Fatalf("预期状态 %d，实际 %v", want, err)
	}
}

func TestCommitFailurePreservesConfigAndConnections(t *testing.T) {
	s, repo := setup(t)
	target := echo(t)
	port := freePort(t)
	in := input(port, target)
	r, e := s.Save(context.Background(), "", 0, in)
	if e != nil {
		t.Fatal(e)
	}
	c, e := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	exchange(t, c, "提交前")
	next := freePort(t)
	in.ListenPort = next
	repo.fail = true
	_, e = s.Save(context.Background(), r.ID, r.Revision, in)
	status(t, e, 500)
	exchange(t, c, "失败后仍可转发")
	if got := s.List()[0]; got.Revision != r.Revision || got.ListenPort != port {
		t.Fatalf("失败请求改变配置：%+v", got)
	}
	persisted, e := repo.List(context.Background())
	if e != nil || persisted[0].ListenPort != port {
		t.Fatalf("数据库变化：%v %v", persisted, e)
	}
	l, e := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(next)))
	if e != nil {
		t.Fatalf("预备监听泄漏：%v", e)
	}
	l.Close()
	status(t, s.Delete(context.Background(), r.ID, r.Revision), 500)
	exchange(t, c, "删除失败仍可转发")
	repo.fail = false
	in.ListenPort = port
	in.Name = "仅改名称"
	r, e = s.Save(context.Background(), r.ID, r.Revision, in)
	if e != nil {
		t.Fatal(e)
	}
	exchange(t, c, "名称不影响会话")
	_, e = s.Save(context.Background(), r.ID, r.Revision-1, in)
	status(t, e, 412)
	if e = s.Delete(context.Background(), r.ID, r.Revision); e != nil {
		t.Fatal(e)
	}
	c.SetReadDeadline(time.Now().Add(time.Second))
	if n, e := c.Read(make([]byte, 1)); n != 0 || e == nil {
		t.Fatal("删除后原连接未被关闭")
	}
	l, e = net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if e != nil {
		t.Fatalf("删除后端口未释放：%v", e)
	}
	l.Close()
}

func TestOccupiedPortAndValidation(t *testing.T) {
	s, _ := setup(t)
	target := echo(t)
	r, e := s.Save(context.Background(), "", 0, input(freePort(t), target))
	if e != nil {
		t.Fatal(e)
	}
	occupied, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	in := input(occupied.Addr().(*net.TCPAddr).Port, target)
	_, e = s.Save(context.Background(), r.ID, r.Revision, in)
	status(t, e, 409)
	if s.List()[0].Revision != r.Revision {
		t.Fatal("绑定失败改变了版本")
	}
	for _, mutate := range []func(*model.Input){func(i *model.Input) { i.ListenPort = 0 }, func(i *model.Input) { i.TargetPort = 65536 }, func(i *model.Input) { i.TargetHost = "https://example.com/path" }, func(i *model.Input) { i.ListenHost = "localhost" }, func(i *model.Input) { i.Name = " " }} {
		in = input(freePort(t), target)
		mutate(&in)
		_, e = s.Save(context.Background(), "", 0, in)
		status(t, e, 400)
	}
	_, e = s.Save(context.Background(), "", 0, input(8080, target))
	status(t, e, 409)
	in = input(r.ListenPort, target)
	in.Protocol = "udp"
	other, e := s.Save(context.Background(), "", 0, in)
	if e != nil || other.Status != "running" {
		t.Fatalf("同端口不同协议不能共存：%v", e)
	}
}

func TestRestoreIsolatesFailedRule(t *testing.T) {
	db, e := store.Open(filepath.Join(testutil.TempDir(t), "restore.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	engine := forward.New(forward.Limits{})
	s, e := rules.New(context.Background(), db, engine, "127.0.0.1:8080")
	if e != nil {
		t.Fatal(e)
	}
	target := echo(t)
	r1, e := s.Save(context.Background(), "", 0, input(freePort(t), target))
	if e != nil {
		t.Fatal(e)
	}
	r2, e := s.Save(context.Background(), "", 0, input(freePort(t), target))
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	block, e := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(r1.ListenPort)))
	if e != nil {
		t.Fatal(e)
	}
	restored, e := rules.New(context.Background(), db, engine, "127.0.0.1:8080")
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	for _, r := range restored.List() {
		if r.ID == r1.ID && r.Status != "error" {
			t.Fatal("占用规则没有显示错误")
		}
		if r.ID == r2.ID && r.Status != "running" {
			t.Fatal("一条错误阻断了其他规则")
		}
	}
	c, e := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(r2.ListenPort)))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	exchange(t, c, "恢复后转发")
	block.Close()
	retried, e := restored.Retry(r1.ID, r1.Revision)
	if e != nil || retried.Status != "running" {
		t.Fatalf("重试失败：%v", e)
	}
}
