//go:build linux

package localapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/native"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

type faultStore struct {
	*store.Store
	fail       atomic.Bool
	saveStart  chan struct{}
	saveResume <-chan struct{}
}

func (f *faultStore) Save(ctx context.Context, v model.Rule, create bool) error {
	if f.saveStart != nil {
		close(f.saveStart)
		<-f.saveResume
	}
	if f.fail.Load() {
		return errors.New("注入保存失败")
	}
	return f.Store.Save(ctx, v, create)
}
func (f *faultStore) Delete(ctx context.Context, id string) error {
	if f.fail.Load() {
		return errors.New("注入删除失败")
	}
	return f.Store.Delete(ctx, id)
}

type fixture struct {
	service *rules.Service
	repo    *faultStore
	admin   model.User
	path    string
	server  *Server
	client  *Client
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dir := testutil.TempDir(t)
	for parent := dir; ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil {
			t.Fatal(err)
		}
		if !trustedDirectory(info) {
			t.Skipf("真实套接字测试需要所有祖先可信的隔离目录，当前路径不满足：%s", parent)
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	db, err := store.Open(filepath.Join(dir, "rules.db"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{repo: &faultStore{Store: db}, path: filepath.Join(dir, "control.sock")}
	f.admin, err = db.EnsureAdmin(context.Background(), "管理员")
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = rules.New(context.Background(), f.repo, forward.New(forward.Limits{}), "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	f.server, err = start(f.path, f.service, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	f.client = NewClient(f.path)
	t.Cleanup(func() {
		if err := f.server.Close(); err != nil {
			t.Error(err)
		}
		f.service.Close()
		db.Close()
	})
	return f
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("错误=%v，期望 %s", err, want)
	}
}
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root 正向测试需要隔离用户命名空间内的真实 UID 0")
	}
}
func testInput(protocol string, port, target int) model.AdminRuleCreateInput {
	return model.AdminRuleCreateInput{Input: model.Input{Name: "测试转发", Protocol: protocol, ListenHost: "127.0.0.1", ListenPort: port, TargetHost: "127.0.0.1", TargetPort: target}}
}
func freePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type directoryMetadata struct {
	os.FileInfo
	mode  os.FileMode
	owner any
}

func (v directoryMetadata) Mode() os.FileMode { return v.mode }
func (v directoryMetadata) IsDir() bool       { return v.mode.IsDir() }
func (v directoryMetadata) Sys() any          { return v.owner }

func TestControlDirectoryTrustIncludesReadOnlyOwners(t *testing.T) {
	foreign := uint32(os.Geteuid()) + 1
	for _, tc := range []struct {
		name string
		mode os.FileMode
		uid  uint32
		want bool
	}{
		{"root可写", os.ModeDir | 0755, 0, true},
		{"服务用户可写", os.ModeDir | 0700, uint32(os.Geteuid()), true},
		{"服务用户只读", os.ModeDir | 0555, uint32(os.Geteuid()), true},
		{"其他用户可写", os.ModeDir | 0755, foreign, false},
		{"其他用户只读仍可改权限", os.ModeDir | 0555, foreign, false},
		{"组可写", os.ModeDir | 0770, 0, false},
		{"所有人可写", os.ModeDir | 0777, 0, false},
		{"符号链接", os.ModeSymlink | 0777, 0, false},
		{"普通文件", 0600, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := directoryMetadata{mode: tc.mode, owner: &syscall.Stat_t{Uid: tc.uid}}
			if trustedDirectory(info) != tc.want {
				t.Fatal("目录可信判断不符合预期")
			}
		})
	}
	if trustedDirectory(directoryMetadata{mode: os.ModeDir | 0700}) {
		t.Fatal("无法确认归属的目录被当作可信目录")
	}
}
func asInput(v model.Rule) model.Input {
	return model.Input{Name: v.Name, Protocol: v.Protocol, ListenHost: v.ListenHost, ListenPort: v.ListenPort, TargetHost: v.TargetHost, TargetPort: v.TargetPort, Enabled: &v.Enabled, TrafficEnabled: &v.TrafficEnabled}
}
func TestPeerCredentialsRejectNonRootAndForgedHeaders(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("非 root 真实对端用例在常规测试进程运行")
	}
	f := setup(t)
	code(t, func() error { _, e := f.client.List(context.Background(), ""); return e }(), "forbidden")
	req, _ := http.NewRequest("GET", "http://localhost/v1/rules", nil)
	req.Header.Set("X-User-ID", model.AdminID)
	req.Header.Set("X-Role", "admin")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	resp, err := f.client.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("请求头绕过了真实对端授权")
	}
	if len(f.service.ListFor(f.admin, "")) != 0 {
		t.Fatal("拒绝请求改变了规则")
	}
}
func TestSocketLifecycleAndPathConflicts(t *testing.T) {
	f := setup(t)
	info, err := os.Stat(f.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("权限错误：%v %v", info, err)
	}
	if s, err := start(f.path, f.service, f.admin); err == nil {
		s.Close()
		t.Fatal("接管了活动套接字")
	}
	if err = f.server.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.server.Close(); err != nil {
		t.Fatal("关闭不幂等")
	}
	if _, err = os.Lstat(f.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("退出未清理节点")
	}
	for _, kind := range []string{"file", "symlink", "stale"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(testutil.TempDir(t), "control.sock")
			switch kind {
			case "file":
				if err := os.WriteFile(path, []byte("保留"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(f.path, path); err != nil {
					t.Fatal(err)
				}
			case "stale":
				l, e := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
				if e != nil {
					t.Fatal(e)
				}
				l.SetUnlinkOnClose(false)
				l.Close()
			}
			s, err := start(path, f.service, f.admin)
			if kind == "stale" {
				if err != nil {
					t.Fatal(err)
				}
				s.Close()
			} else {
				if err == nil {
					s.Close()
					t.Fatal("未拒绝冲突节点")
				}
				if _, err = os.Lstat(path); err != nil {
					t.Fatal("误删冲突节点")
				}
			}
		})
	}
	unsafe := filepath.Join(testutil.TempDir(t), "unsafe")
	if err = os.Mkdir(unsafe, 0777); err != nil {
		t.Fatal(err)
	}
	os.Chmod(unsafe, 0777)
	if s, err := start(filepath.Join(unsafe, "control.sock"), f.service, f.admin); err == nil {
		s.Close()
		t.Fatal("接受了其他用户可写目录")
	}
	t.Run("替换节点不误删", func(t *testing.T) {
		path := filepath.Join(testutil.TempDir(t), "control.sock")
		s, err := start(path, f.service, f.admin)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte("替换"), 0600); err != nil {
			t.Fatal(err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "替换" {
			t.Fatal("误删替换文件")
		}
	})
}
func TestNativeBoundary(t *testing.T) {
	if native.Available() {
		t.Skip("此环境支持本机管理")
	}
	path := filepath.Join(testutil.TempDir(t), "disabled.sock")
	s, err := Start(path, nil, model.User{})
	if err != nil || s != nil {
		t.Fatalf("禁用环境创建了控制入口：%v %v", s, err)
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("禁用环境留下节点")
	}
}
func TestRootCRUDForwardingAndRestart(t *testing.T) {
	requireRoot(t)
	f := setup(t)
	ctx := context.Background()
	for _, protocol := range []string{"tcp", "udp", "tcp+udp"} {
		t.Run(protocol, func(t *testing.T) {
			target, stop := echoPair(t)
			defer stop()
			port := freePort(t)
			value, err := f.client.Create(ctx, testInput(protocol, port, target))
			if err != nil {
				t.Fatal(err)
			}
			if value.OwnerID != model.AdminID || value.Owner == nil || value.Owner.Username != "管理员" || !value.Enabled || value.TrafficEnabled {
				t.Fatalf("创建默认值错误：%+v", value)
			}
			listed, err := f.client.List(ctx, model.AdminID)
			if err != nil || len(listed) != 1 {
				t.Fatalf("列表错误：%v %v", listed, err)
			}
			for _, transport := range model.Transports(protocol) {
				exchange(t, transport, port)
			}
			before, err := f.client.Get(ctx, value.ID)
			if err != nil {
				t.Fatal(err)
			}
			in := asInput(before)
			in.Name = "新名称"
			changed, err := f.client.Update(ctx, before.ID, before.Revision, in)
			if err != nil {
				t.Fatal(err)
			}
			if changed.Revision != before.Revision+1 || changed.OwnerID != before.OwnerID {
				t.Fatal("修改未保持归属/版本")
			}
			_, err = f.client.Update(ctx, before.ID, before.Revision, in)
			code(t, err, "revision_mismatch")
			code(t, f.client.Delete(ctx, before.ID, before.Revision), "revision_mismatch")
			f.repo.fail.Store(true)
			in.ListenPort = freePort(t)
			_, err = f.client.Update(ctx, changed.ID, changed.Revision, in)
			code(t, err, "database_error")
			code(t, f.client.Delete(ctx, changed.ID, changed.Revision), "database_error")
			f.repo.fail.Store(false)
			for _, transport := range model.Transports(protocol) {
				exchange(t, transport, port)
			}
			// 服务重建从同一数据库恢复已提交配置与监听。
			if err = f.server.Close(); err != nil {
				t.Fatal(err)
			}
			if err = f.service.Close(); err != nil {
				t.Fatal(err)
			}
			f.service, err = rules.New(ctx, f.repo, forward.New(forward.Limits{}), "127.0.0.1:8080")
			if err != nil {
				t.Fatal(err)
			}
			f.server, err = start(f.path, f.service, f.admin)
			if err != nil {
				t.Fatal(err)
			}
			for _, transport := range model.Transports(protocol) {
				exchange(t, transport, port)
			}
			if err = f.client.Delete(ctx, changed.ID, changed.Revision); err != nil {
				t.Fatal(err)
			}
			_, err = f.client.Get(ctx, changed.ID)
			code(t, err, "not_found")
			if len(f.service.ListFor(f.admin, "")) != 0 {
				t.Fatal("删除未同步服务")
			}
		})
	}
}
func echoPair(t *testing.T) (int, func()) {
	t.Helper()
	var tcp net.Listener
	var udp net.PacketConn
	var err error
	for i := 0; i < 100; i++ {
		tcp, err = net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		udp, err = net.ListenPacket("udp4", tcp.Addr().String())
		if err == nil {
			break
		}
		tcp.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, e := tcp.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	go func() {
		b := make([]byte, 4096)
		for {
			n, a, e := udp.ReadFrom(b)
			if e != nil {
				return
			}
			udp.WriteTo(b[:n], a)
		}
	}()
	return tcp.Addr().(*net.TCPAddr).Port, func() { tcp.Close(); udp.Close() }
}
func exchange(t *testing.T, transport string, port int) {
	t.Helper()
	c, e := net.DialTimeout(transport, fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	c.Write([]byte("真实转发"))
	b := make([]byte, len("真实转发"))
	if _, e = io.ReadFull(c, b); e != nil || string(b) != "真实转发" {
		t.Fatalf("转发失败：%q %v", b, e)
	}
}
func TestRootConcurrentRevisionAndStrictInput(t *testing.T) {
	requireRoot(t)
	f := setup(t)
	ctx := context.Background()
	in := testInput("tcp+udp", freePort(t), 1)
	disabled := false
	in.Enabled = &disabled
	old, err := f.client.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := asInput(old)
			input.Name = fmt.Sprint(i)
			_, e := f.client.Update(ctx, old.ID, old.Revision, input)
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			code(t, e, "revision_mismatch")
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("并发覆盖：%d/%d", success, conflicts)
	}
	for _, body := range []string{`{"unknown":true}`, `{"owner_id":"other"}`, `{} {}`, strings.Repeat(" ", 65536) + `{}`} {
		req, _ := http.NewRequest("PUT", "http://localhost/v1/rules/"+old.ID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", `"2"`)
		resp, e := f.client.http.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("非法输入未拒绝：%d", resp.StatusCode)
		}
	}
	var out model.Rule
	code(t, f.client.request(ctx, "PUT", resource+"/"+old.ID, 0, in.Input, &out), "revision_required")
	_, err = f.client.Get(ctx, "missing")
	code(t, err, "not_found")
	bad := testInput("tcp", freePort(t), 80)
	bad.ListenHost = "invalid"
	_, err = f.client.Create(ctx, bad)
	code(t, err, "validation_error")
}

func TestRootCloseWaitsForPendingControlWriteAndConcurrentRead(t *testing.T) {
	requireRoot(t)
	f := setup(t)
	started, resume := make(chan struct{}), make(chan struct{})
	f.repo.saveStart, f.repo.saveResume = started, resume
	var once sync.Once
	unblock := func() { once.Do(func() { close(resume) }) }
	defer unblock()
	created := make(chan error, 1)
	in := testInput("tcp", freePort(t), 1)
	disabled := false
	in.Enabled = &disabled
	go func() { _, err := f.client.Create(context.Background(), in); created <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("控制写操作未进入持久化")
	}
	// Web 与本机控制读取共用服务锁；关闭必须等待已接受的写操作完成。
	read := make(chan []model.Rule, 1)
	go func() { read <- f.service.ListFor(f.admin, "") }()
	closed := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { closed <- f.server.Close() }()
	}
	select {
	case err := <-closed:
		t.Fatalf("在途写操作完成前关闭返回：%v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-created:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("在途创建未完成")
	}
	select {
	case items := <-read:
		if len(items) != 1 || items[0].Name != in.Name {
			t.Fatal("并发读取未看到已提交规则")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("并发读取未退出")
	}
	for i := 0; i < 2; i++ {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("并发关闭未退出")
		}
	}
}

func TestRootOwnerConstraints(t *testing.T) {
	requireRoot(t)
	f := setup(t)
	ctx := context.Background()
	_, err := f.service.SaveSettings(ctx, f.admin, 1, model.SettingsInput{PortStart: 21000, PortEnd: 21010})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.service.CreatePlan(ctx, f.admin, model.PlanInput{Name: "测试套餐", PortCount: 2, TrafficLimitBytes: 100000, AccountingMode: "both", ResetMode: "calendar_month"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.service.CreateUser(ctx, f.admin, model.UserInput{Username: "测试用户", Password: "测试密码123", PlanID: plan.ID})
	if err != nil {
		t.Fatal(err)
	}
	in := testInput("tcp", account.User.PortStart, 1)
	in.OwnerID = account.User.ID
	disabled := false
	in.Enabled = &disabled
	value, err := f.client.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if value.OwnerID != account.User.ID || value.Owner == nil || value.Owner.Username != account.User.Username {
		t.Fatal("未返回权威归属")
	}
	selected, err := f.client.List(ctx, account.User.ID)
	if err != nil || len(selected) != 1 {
		t.Fatal("归属筛选失败")
	}
	input := asInput(value)
	input.ListenPort = account.User.PortEnd + 1
	_, err = f.client.Update(ctx, value.ID, value.Revision, input)
	code(t, err, "validation_error")
	in.OwnerID = model.AdminID
	_, err = f.client.Create(ctx, in)
	code(t, err, "port_reserved")
	in.OwnerID = "missing"
	_, err = f.client.Create(ctx, in)
	code(t, err, "not_found")
}
