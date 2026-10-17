package rules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

type quotaFaultRepo struct {
	*store.Store
	failReset     atomic.Bool
	failResetUser atomic.Value
	failSave      atomic.Bool
	blockReset    chan struct{}
	resetEntered  chan struct{}
}

func (r *quotaFaultRepo) ResetUsage(ctx context.Context, v model.Usage) error {
	failedUser, _ := r.failResetUser.Load().(string)
	if r.failReset.Load() || failedUser == v.UserID {
		return errors.New("模拟周期提交故障")
	}
	if r.blockReset != nil {
		select {
		case r.resetEntered <- struct{}{}:
		default:
		}
		<-r.blockReset
	}
	return r.Store.ResetUsage(ctx, v)
}
func (r *quotaFaultRepo) SaveUsages(ctx context.Context, v []model.Usage) error {
	if r.failSave.Load() {
		return errors.New("模拟账本保存故障")
	}
	return r.Store.SaveUsages(ctx, v)
}

type quotaFixture struct {
	s     *Service
	repo  *quotaFaultRepo
	admin model.User
	clock atomic.Int64
	base  int
	path  string
}

func (q *quotaFixture) now() time.Time { return time.Unix(0, q.clock.Load()).UTC() }
func newQuotaFixture(t *testing.T) *quotaFixture {
	t.Helper()
	q := &quotaFixture{}
	q.clock.Store(time.Date(2026, 1, 31, 23, 59, 50, 0, time.UTC).UnixNano())
	q.path = filepath.Join(testutil.TempDir(t), "quota.db")
	db, err := store.Open(q.path)
	if err != nil {
		t.Fatal(err)
	}
	q.repo = &quotaFaultRepo{Store: db}
	q.admin, err = db.EnsureAdmin(context.Background(), "管理员")
	if err != nil {
		t.Fatal(err)
	}
	q.s, err = New(context.Background(), q.repo, forward.New(forward.Limits{}), "127.0.0.1:8080", Options{FlushInterval: time.Hour, Now: q.now})
	if err != nil {
		t.Fatal(err)
	}
	q.base = availableRange(t, 16)
	_, err = q.s.SaveSettings(context.Background(), q.admin, 1, model.SettingsInput{PortStart: q.base, PortEnd: q.base + 15})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.s.Close(); db.Close() })
	return q
}
func availableRange(t *testing.T, count int) int {
	t.Helper()
	for tries := 0; tries < 200; tries++ {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		base := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if base+count > 65535 {
			continue
		}
		var all []io.Closer
		ok := true
		for p := base; p < base+count; p++ {
			tcp, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", p))
			if e != nil {
				ok = false
				break
			}
			all = append(all, tcp)
			udp, e := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", p))
			if e != nil {
				ok = false
				break
			}
			all = append(all, udp)
		}
		for _, c := range all {
			c.Close()
		}
		if ok {
			return base
		}
	}
	t.Fatal("找不到可用连续端口")
	return 0
}
func (q *quotaFixture) user(t *testing.T, name, mode string, limit int64) model.User {
	t.Helper()
	p, err := q.s.CreatePlan(context.Background(), q.admin, model.PlanInput{Name: name, PortCount: 4, TrafficLimitBytes: limit, AccountingMode: mode, ResetMode: model.ResetCalendarMonth})
	if err != nil {
		t.Fatal(err)
	}
	a, err := q.s.CreateUser(context.Background(), q.admin, model.UserInput{Username: name, Password: "测试密码123", PlanID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	return a.User
}
func (q *quotaFixture) rule(t *testing.T, u model.User, port, target int, protocol string, enabled bool) model.Rule {
	t.Helper()
	r, err := q.s.SaveFor(context.Background(), q.admin, u.ID, "", 0, model.Input{Name: "配额转发", Protocol: protocol, ListenHost: "127.0.0.1", ListenPort: port, TargetHost: "127.0.0.1", TargetPort: target, Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func targetPair(t *testing.T, request, response int) int {
	t.Helper()
	base := availableRange(t, 1)
	tcp, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", base))
	if err != nil {
		t.Fatal(err)
	}
	udp, err := net.ListenPacket("udp4", tcp.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	acceptDone := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var conns []net.Conn
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer close(acceptDone)
		for {
			c, e := tcp.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				b := make([]byte, request)
				for {
					if _, err := io.ReadFull(c, b); err != nil {
						return
					}
					if _, err := c.Write(make([]byte, response)); err != nil {
						return
					}
				}
			}()
		}
	}()
	go func() {
		defer wg.Done()
		b := make([]byte, 65535)
		for {
			_, peer, e := udp.ReadFrom(b)
			if e != nil {
				return
			}
			if _, e = udp.WriteTo(make([]byte, response), peer); e != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		tcp.Close()
		udp.Close()
		<-acceptDone
		mu.Lock()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return base
}
func quotaTCP(t *testing.T, port int) net.Conn {
	t.Helper()
	c, e := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func sendTCP(t *testing.T, c net.Conn, up, down int) {
	t.Helper()
	c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write(make([]byte, up)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(c, make([]byte, down)); err != nil {
		t.Fatal(err)
	}
}
func sendUDP(t *testing.T, port, up, down int) {
	t.Helper()
	c, e := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", port))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	if _, e = c.Write(make([]byte, up)); e != nil {
		t.Fatal(e)
	}
	b := make([]byte, 65535)
	if n, e := c.Read(b); e != nil || n != down {
		t.Fatalf("UDP 实际响应 %d: %v", n, e)
	}
}
func eventuallyQuota(t *testing.T, fn func() bool) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("配额运行态未按预期变化")
}

func TestQuotaAccountingAggregatesAcrossRulesAndProtocols(t *testing.T) {
	for mode, want := range map[string]int64{model.AccountingBoth: 220, model.AccountingMax: 110, model.AccountingUpload: 110, model.AccountingDownload: 110} {
		t.Run(mode, func(t *testing.T) {
			q := newQuotaFixture(t)
			u := q.user(t, "用户", mode, 100000)
			a := targetPair(t, 100, 10)
			b := targetPair(t, 10, 100)
			first := q.rule(t, u, u.PortStart, a, "tcp+udp", true)
			q.rule(t, u, u.PortStart+1, b, "tcp+udp", true)
			c := quotaTCP(t, u.PortStart)
			sendTCP(t, c, 100, 10)
			sendUDP(t, u.PortStart+1, 10, 100)
			eventuallyQuota(t, func() bool { return q.s.meters[u.ID].Snapshot().DownloadBytes == 110 })
			v := q.s.meters[u.ID].Snapshot()
			if v.UploadBytes != 110 || v.DownloadBytes != 110 || v.UsedBytes != want {
				t.Fatalf("全用户聚合错误：%+v", v)
			}
			for _, r := range q.s.List() {
				if r.Traffic.UploadBytes != 0 || r.Traffic.DownloadBytes != 0 {
					t.Fatal("展示关闭时仍统计了展示流量")
				}
			}
			if err := q.s.DeleteFor(context.Background(), q.admin, first.ID, first.Revision); err != nil {
				t.Fatal(err)
			}
			if q.s.meters[u.ID].Snapshot().UsedBytes != want {
				t.Fatal("删除规则退还套餐额度")
			}
			if err := q.s.Close(); err != nil {
				t.Fatal(err)
			}
			persisted, err := q.repo.ListUsages(context.Background())
			if err != nil || len(persisted) != 1 || persisted[0].UsedBytes != want {
				t.Fatalf("最终账本未保存：%+v %v", persisted, err)
			}
		})
	}
}
func TestQuotaExhaustionActivelyStopsAllRulesAndPreservesOtherUser(t *testing.T) {
	for _, mode := range []string{model.AccountingBoth, model.AccountingMax, model.AccountingUpload, model.AccountingDownload} {
		t.Run(mode, func(t *testing.T) {
			q := newQuotaFixture(t)
			u := q.user(t, "耗尽用户", mode, 2000)
			other := q.user(t, "其他用户", mode, 100000)
			target := targetPair(t, 1000, 1000)
			first := q.rule(t, u, u.PortStart, target, "tcp+udp", true)
			q.rule(t, u, u.PortStart+1, target, "tcp+udp", true)
			q.rule(t, other, other.PortStart, target, "tcp+udp", true)
			c := quotaTCP(t, u.PortStart+1)
			sendTCP(t, c, 1000, 1000)
			// 后续一次真实写入达到额度；不借助列表或账户请求驱动停止。
			if mode != model.AccountingBoth {
				udp, _ := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", u.PortStart))
				udp.SetDeadline(time.Now().Add(200 * time.Millisecond))
				udp.Write(make([]byte, 1000))
				udp.Read(make([]byte, 65535))
				udp.Close()
			}
			eventuallyQuota(t, func() bool {
				for _, p := range []int{u.PortStart, u.PortStart + 1} {
					probe, e := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", p), 30*time.Millisecond)
					if e == nil {
						probe.Close()
						return false
					}
				}
				return true
			})
			c.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := c.Read(make([]byte, 1)); err == nil {
				t.Fatal("超额后的已有TCP连接仍可读")
			} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				t.Fatal("超额后已有连接未主动关闭")
			}
			eventuallyQuota(t, func() bool {
				for _, p := range []int{u.PortStart, u.PortStart + 1} {
					probe, e := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", p))
					if e != nil {
						return false
					}
					probe.Close()
				}
				return true
			})
			if _, err := q.s.RetryFor(q.admin, first.ID, first.Revision); err == nil {
				t.Fatal("管理员重试绕过用户额度")
			}
			sendTCP(t, quotaTCP(t, other.PortStart), 1000, 1000)
			sendUDP(t, other.PortStart, 1000, 1000)
			for _, r := range q.s.ListFor(q.admin, u.ID) {
				if !r.Enabled || r.Status != "quota_exceeded" {
					t.Fatalf("额度暂停改变启用意图：%+v", r)
				}
			}
		})
	}
}
func TestQuotaCycleRestoresOnlyPausedRulesAndDoesNotDropHealthyConnections(t *testing.T) {
	q := newQuotaFixture(t)
	u := q.user(t, "正常用户", model.AccountingBoth, 100000)
	target := targetPair(t, 100, 100)
	q.rule(t, u, u.PortStart, target, "tcp+udp", true)
	c := quotaTCP(t, u.PortStart)
	sendTCP(t, c, 100, 100)
	paused := q.user(t, "耗尽用户", model.AccountingBoth, 200)
	r := q.rule(t, paused, paused.PortStart, target, "tcp", true)
	disabled := q.rule(t, paused, paused.PortStart+1, target, "tcp", false)
	deleted := q.rule(t, paused, paused.PortStart+2, target, "tcp", true)
	if err := q.s.DeleteFor(context.Background(), q.admin, deleted.ID, deleted.Revision); err != nil {
		t.Fatal(err)
	}
	manual := q.rule(t, paused, paused.PortStart+3, target, "tcp+udp", true)
	pc := quotaTCP(t, paused.PortStart)
	sendTCP(t, pc, 100, 100)
	eventuallyQuota(t, func() bool { q.s.mu.Lock(); defer q.s.mu.Unlock(); return q.s.running[r.ID] == nil })
	enabled := false
	if _, err := q.s.SaveFor(context.Background(), q.admin, "", manual.ID, manual.Revision, model.Input{Name: manual.Name, Protocol: manual.Protocol, ListenHost: manual.ListenHost, ListenPort: manual.ListenPort, TargetHost: manual.TargetHost, TargetPort: manual.TargetPort, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	q.clock.Store(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	// 由下一次数据准入驱动周期转换，原连接保持可用。
	sendTCP(t, c, 100, 100)
	eventuallyQuota(t, func() bool { q.s.mu.Lock(); defer q.s.mu.Unlock(); return q.s.running[r.ID] != nil })
	q.s.mu.Lock()
	revived := q.s.running[disabled.ID] != nil || q.s.running[deleted.ID] != nil || q.s.running[manual.ID] != nil
	q.s.mu.Unlock()
	if revived {
		t.Fatal("周期转换复活手动停用或删除规则")
	}
	v := q.s.meters[u.ID].Snapshot()
	if v.UploadBytes != 100 || v.DownloadBytes != 100 || !v.CycleStart.Equal(q.now()) {
		t.Fatalf("新周期用量错误：%+v", v)
	}
}
func TestQuotaCycleFailureBlocksAdmissionAndRetainsInFlightNewCycleBytes(t *testing.T) {
	q := newQuotaFixture(t)
	u := q.user(t, "周期用户", model.AccountingMax, 100000)
	m := q.s.meters[u.ID]
	target := targetPair(t, 100, 100)
	q.rule(t, u, u.PortStart, target, "tcp", true)
	c := quotaTCP(t, u.PortStart)
	sendTCP(t, c, 100, 100)
	eventuallyQuota(t, func() bool { return m.Snapshot().UploadBytes == 100 && m.Snapshot().DownloadBytes == 100 })
	q.repo.failReset.Store(true)
	q.clock.Store(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	m.Add(true, 17)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := m.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("失败重置提前开放额度：%v", err)
	}
	if !m.Snapshot().CycleStart.Before(q.now()) {
		t.Fatal("失败周期提交提前清零")
	}
	q.repo.failReset.Store(false)
	q.s.signalQuota()
	sendTCP(t, c, 100, 100)
	v := m.Snapshot()
	if v.UploadBytes != 117 || v.DownloadBytes != 100 {
		t.Fatalf("暂存的跨边界用量丢失：%+v", v)
	}
}
func TestQuotaPortOwnershipAndConcurrentUserAllocation(t *testing.T) {
	q := newQuotaFixture(t)
	u := q.user(t, "范围用户", model.AccountingBoth, 100000)
	target := targetPair(t, 1, 1)
	enabled := false
	in := model.Input{Name: "端口", Protocol: "udp", ListenHost: "127.0.0.2", ListenPort: u.PortStart, TargetHost: "127.0.0.1", TargetPort: target, Enabled: &enabled}
	if _, err := q.s.SaveFor(context.Background(), q.admin, q.admin.ID, "", 0, in); err == nil {
		t.Fatal("管理员不同IP/协议抢占用户数字端口")
	}
	in.ListenPort = u.PortEnd + 1
	if _, err := q.s.SaveFor(context.Background(), u, "", "", 0, in); err == nil {
		t.Fatal("普通用户越过分配范围")
	}
	p, err := q.s.CreatePlan(context.Background(), q.admin, model.PlanInput{Name: "并发", PortCount: 4, TrafficLimitBytes: 100, AccountingMode: model.AccountingBoth, ResetMode: model.ResetCalendarMonth})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan model.Account, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a, e := q.s.CreateUser(context.Background(), q.admin, model.UserInput{Username: "并发" + strconv.Itoa(i), Password: "测试密码123", PlanID: p.ID})
			if e != nil {
				errs <- e
			} else {
				results <- a
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	occupied := map[int]bool{}
	count := 0
	for a := range results {
		count++
		for port := a.User.PortStart; port <= a.User.PortEnd; port++ {
			if occupied[port] {
				t.Fatal("并发分配重叠")
			}
			occupied[port] = true
		}
	}
	if count != 3 || len(errs) != 1 {
		t.Fatalf("连续池用尽未原子回滚：成功 %d，失败 %d", count, len(errs))
	}
	users, err := q.repo.ListUsers(context.Background())
	if err != nil || len(users) != 5 {
		t.Fatalf("失败用户残留：%d %v", len(users), err)
	}
}

func TestQuotaCommitAcrossTwoBoundariesKeepsCASProgress(t *testing.T) {
	q := newQuotaFixture(t)
	u := q.user(t, "跨期用户", model.AccountingMax, 100000)
	m := q.s.meters[u.ID]
	q.repo.blockReset = make(chan struct{})
	q.repo.resetEntered = make(chan struct{}, 2)
	q.clock.Store(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	m.Add(true, 11)
	select {
	case <-q.repo.resetEntered:
	case <-time.After(time.Second):
		t.Fatal("未进入阻塞周期事务")
	}
	q.clock.Store(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	m.Add(true, 23)
	close(q.repo.blockReset)
	eventuallyQuota(t, func() bool { v := m.Snapshot(); return !m.pending() && v.CycleStart.Equal(q.now()) })
	v := m.Snapshot()
	if v.Version != 3 || v.UploadBytes != 23 {
		t.Fatalf("跨两次边界后丢失最新增量或版本：%+v", v)
	}
	if err := q.s.Close(); err != nil {
		t.Fatal(err)
	}
	values, err := q.repo.ListUsages(context.Background())
	if err != nil || values[0].Version != 3 || values[0].UploadBytes != 23 {
		t.Fatalf("数据库CAS未收敛：%+v %v", values, err)
	}
}
func TestQuotaSaveFailureRetriesAndRestartNeverRefunds(t *testing.T) {
	q := newQuotaFixture(t)
	u := q.user(t, "账本用户", model.AccountingMax, 200)
	target := targetPair(t, 100, 100)
	r := q.rule(t, u, u.PortStart, target, "tcp+udp", true)
	c := quotaTCP(t, u.PortStart)
	sendTCP(t, c, 100, 100)
	m := q.s.meters[u.ID]
	eventuallyQuota(t, func() bool { return m.Snapshot().DownloadBytes == 100 })
	q.repo.failSave.Store(true)
	q.s.mu.Lock()
	err := q.s.flushUsages(context.Background())
	q.s.mu.Unlock()
	if err == nil {
		t.Fatal("保存故障未上报")
	}
	if m.Snapshot().UsedBytes != 100 {
		t.Fatal("保存失败丢内存增量")
	}
	q.repo.failSave.Store(false)
	if _, err = q.s.ResetTrafficFor(context.Background(), q.admin, r.ID, r.Revision); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().UsedBytes != 100 {
		t.Fatal("展示清零退还套餐额度")
	}
	c.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := c.Write(make([]byte, 100)); err != nil {
		t.Fatal(err)
	}
	eventuallyQuota(t, func() bool { q.s.mu.Lock(); defer q.s.mu.Unlock(); return q.s.running[r.ID] == nil })
	if err := q.s.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := New(context.Background(), q.repo, forward.New(forward.Limits{}), "127.0.0.1:8080", Options{Now: q.now})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	values := again.ListFor(q.admin, u.ID)
	if len(values) != 1 || values[0].Status != "quota_exceeded" || !values[0].Enabled {
		t.Fatalf("重启绕过额度：%+v", values)
	}
	q.clock.Store(time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).UnixNano())
	again.signalQuota()
	eventuallyQuota(t, func() bool { again.mu.Lock(); defer again.mu.Unlock(); return again.running[r.ID] != nil })
	got := again.meters[u.ID].Snapshot()
	if got.UploadBytes != 0 || got.UsedBytes != 0 || got.CycleStart.Month() != time.May {
		t.Fatalf("停机跨期没有进入当前周期：%+v", got)
	}
}

func TestQuotaFinalSaveErrorIsReportedAndCloseIsIdempotent(t *testing.T) {
	q := newQuotaFixture(t)
	u := q.user(t, "退出用户", model.AccountingBoth, 100000)
	target := targetPair(t, 100, 100)
	q.rule(t, u, u.PortStart, target, "tcp", true)
	sendTCP(t, quotaTCP(t, u.PortStart), 100, 100)
	eventuallyQuota(t, func() bool { return q.s.meters[u.ID].Snapshot().DownloadBytes == 100 })
	q.repo.failSave.Store(true)
	first := q.s.Close()
	second := q.s.Close()
	if first == nil || second != first {
		t.Fatalf("用户账本退出失败没有幂等向上传播：%v %v", first, second)
	}
}
func TestQuotaUserCreationUsesSingleUTCSnapshot(t *testing.T) {
	q := newQuotaFixture(t)
	p, err := q.s.CreatePlan(context.Background(), q.admin, model.PlanInput{Name: "边界套餐", PortCount: 1, TrafficLimitBytes: 100, AccountingMode: model.AccountingBoth, ResetMode: model.ResetCalendarMonth})
	if err != nil {
		t.Fatal(err)
	}
	// 第二次取时已经跨月，开户事务和内存账本仍必须使用同一开户时刻。
	var calls atomic.Int64
	before := time.Date(2026, 1, 31, 23, 59, 59, 999999999, time.UTC)
	q.s.mu.Lock()
	q.s.now = func() time.Time {
		if calls.Add(1) == 1 {
			return before
		}
		return before.Add(time.Nanosecond)
	}
	q.s.mu.Unlock()
	a, err := q.s.CreateUser(context.Background(), q.admin, model.UserInput{Username: "边界开户", Password: "测试密码123", PlanID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	values, err := q.repo.ListUsages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 若后台恰已推进周期，比较对应的数据库版本；未推进时首周期起点必须等于开户时间。
	q.s.mu.Lock()
	v := q.s.meters[a.User.ID].Snapshot()
	q.s.mu.Unlock()
	if v.Version == 1 && (!v.CycleStart.Equal(a.User.CreatedAt) || !values[0].CycleStart.Equal(a.User.CreatedAt)) {
		t.Fatalf("开户首周期不一致：内存 %+v 数据库 %+v", v, values[0])
	}
}

func TestQuotaRestartIsolatesUserCycleFailureAndRecoversOnlyPendingRules(t *testing.T) {
	q := newQuotaFixture(t)
	failed := q.user(t, "启动待重置", model.AccountingBoth, 100000)
	healthy := q.user(t, "启动正常", model.AccountingBoth, 100000)
	target := targetPair(t, 100, 100)
	resume := q.rule(t, failed, failed.PortStart, target, "tcp+udp", true)
	disabled := q.rule(t, failed, failed.PortStart+1, target, "tcp+udp", false)
	manual := q.rule(t, failed, failed.PortStart+2, target, "tcp+udp", true)
	deleted := q.rule(t, failed, failed.PortStart+3, target, "tcp+udp", true)
	q.rule(t, healthy, healthy.PortStart, target, "tcp+udp", true)
	broken := q.rule(t, healthy, healthy.PortStart+1, target, "tcp", true)
	sendTCP(t, quotaTCP(t, failed.PortStart), 100, 100)
	if err := q.s.Close(); err != nil {
		t.Fatal(err)
	}
	q.clock.Store(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	q.repo.failResetUser.Store(failed.ID)
	occupied, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", broken.ListenPort))
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	again, err := New(context.Background(), q.repo, forward.New(forward.Limits{}), "127.0.0.1:8080", Options{Now: q.now, FlushInterval: time.Hour})
	if err != nil {
		t.Fatalf("单用户重置失败阻止整个服务启动：%v", err)
	}
	defer again.Close()
	// 无需账户或规则列表请求，健康用户在启动后立即可以转发。
	c := quotaTCP(t, healthy.PortStart)
	sendTCP(t, c, 100, 100)
	sendUDP(t, healthy.PortStart, 100, 100)
	rejected, e := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", failed.PortStart), 100*time.Millisecond)
	if e == nil {
		rejected.Close()
		t.Fatal("待重置用户在启动时提前恢复TCP")
	}
	udp, e := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", failed.PortStart))
	if e != nil {
		t.Fatalf("待重置用户提前恢复UDP：%v", e)
	}
	udp.Close()
	values := again.ListFor(q.admin, failed.ID)
	for _, r := range values {
		if r.Enabled && (r.Status != "error" || r.LastErrorCode != "quota_reset_pending") {
			t.Fatalf("启动临时暂停被错误标记：%+v", r)
		}
	}
	again.mu.Lock()
	old := again.meters[failed.ID].Snapshot()
	stillPending := again.meters[failed.ID].pending()
	again.mu.Unlock()
	if !stillPending || old.Version != 1 || old.UploadBytes != 100 || old.DownloadBytes != 100 {
		t.Fatalf("失败重置丢失旧账本：%+v", old)
	}
	enabled := false
	_, err = again.SaveFor(context.Background(), q.admin, "", manual.ID, manual.Revision, model.Input{Name: manual.Name, Protocol: manual.Protocol, ListenHost: manual.ListenHost, ListenPort: manual.ListenPort, TargetHost: manual.TargetHost, TargetPort: manual.TargetPort, Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if err := again.DeleteFor(context.Background(), q.admin, deleted.ID, deleted.Revision); err != nil {
		t.Fatal(err)
	}
	occupied.Close()
	q.repo.failResetUser.Store("")
	// 仅靠后台周期重试恢复；保留原来健康用户的活动连接。
	eventuallyQuota(t, func() bool { again.mu.Lock(); defer again.mu.Unlock(); return again.running[resume.ID] != nil })
	sendTCP(t, c, 100, 100)
	sendTCP(t, quotaTCP(t, failed.PortStart), 100, 100)
	sendUDP(t, failed.PortStart, 100, 100)
	again.mu.Lock()
	revived := again.running[disabled.ID] != nil || again.running[manual.ID] != nil || again.running[deleted.ID] != nil || again.running[broken.ID] != nil
	v := again.meters[failed.ID].Snapshot()
	brokenState := again.items[broken.ID]
	again.mu.Unlock()
	if revived || brokenState.LastErrorCode != "listen_failed" {
		t.Fatalf("恢复临时暂停时复活其他规则：%+v", brokenState)
	}
	if v.Version != 2 || !v.CycleStart.Equal(q.now()) {
		t.Fatalf("恢复未按原CAS推进周期：%+v", v)
	}
	// 再跨一周期，之前的临时错误码不能残留并重新准备仍在运行的监听。
	q.clock.Store(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	sendTCP(t, c, 100, 100)
	sendTCP(t, quotaTCP(t, failed.PortStart), 100, 100)
}
