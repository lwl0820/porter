package forward

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

func prepareNamed(t *testing.T, e *Engine, id, protocol string, listenPort int, target net.Addr) *Runtime {
	t.Helper()
	host, portText, err := net.SplitHostPort(target.String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Prepare(model.Rule{ID: id, Protocol: protocol, ListenHost: "127.0.0.1", ListenPort: listenPort, TargetHost: host, TargetPort: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)
	return r
}

func budgetSnapshot(e *Engine, id string) (refs, active int, exists bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.budgets[id]
	if b == nil {
		return 0, 0, false
	}
	return b.refs, b.active, true
}

func tcpExchange(t *testing.T, c *net.TCPConn) {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	var one [1]byte
	if _, err := io.ReadFull(c, one[:]); err != nil || one[0] != 'a' {
		t.Fatalf("TCP 会话收发失败: %v", err)
	}
}

func TestLogicalRuleSharesTCPUDPBudgetAndRecovers(t *testing.T) {
	tcpTarget := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
	udpTarget := testUDPServer(t, "127.0.0.1")
	e := New(Limits{Global: 3, PerRule: 2, UDPIdle: time.Minute})
	tcp := prepareNamed(t, e, "组合规则", "tcp", 0, tcpTarget.Addr())
	udp := prepareNamed(t, e, "组合规则", "udp", tcp.tcp.Addr().(*net.TCPAddr).Port, udpTarget.LocalAddr())
	tcp.Start()
	udp.Start()
	c := dialTCP(t, tcp)
	tcpExchange(t, c)
	u := udpClient(t, udp)
	if err := udpRoundTrip(u, []byte("UDP")); err != nil {
		t.Fatal(err)
	}
	if refs, active, exists := budgetSnapshot(e, "组合规则"); !exists || refs != 2 || active != 2 {
		t.Fatalf("TCP/UDP 没有共享预算: %d %d %v", refs, active, exists)
	}
	rejected := dialTCP(t, tcp)
	var one [1]byte
	if _, err := rejected.Read(one[:]); err == nil {
		t.Fatal("TCP 绕过了组合规则额度")
	}
	secondUDP := udpClient(t, udp)
	_ = secondUDP.SetDeadline(time.Now().Add(40 * time.Millisecond))
	_, _ = secondUDP.Write([]byte("拒绝"))
	if _, err := secondUDP.Read(one[:]); err == nil {
		t.Fatal("UDP 绕过了组合规则额度")
	}
	other := prepareNamed(t, e, "其他规则", "tcp", 0, tcpTarget.Addr())
	other.Start()
	otherClient := dialTCP(t, other)
	tcpExchange(t, otherClient)
	globalRejected := prepareNamed(t, e, "第三条规则", "tcp", 0, tcpTarget.Addr())
	globalRejected.Start()
	if _, err := dialTCP(t, globalRejected).Read(one[:]); err == nil {
		t.Fatal("共享预算改变了全局额度")
	}
	_ = c.Close()
	await(t, func() bool { _, active, _ := budgetSnapshot(e, "组合规则"); return active == 1 })
	if err := udpRoundTrip(secondUDP, []byte("恢复")); err != nil {
		t.Fatalf("TCP 释放后 UDP 没有恢复额度: %v", err)
	}
	tcp.Stop()
	if refs, active, _ := budgetSnapshot(e, "组合规则"); refs != 1 || active != 2 {
		t.Fatalf("停止空 TCP 实例重置了 UDP 额度: %d %d", refs, active)
	}
	udp.Stop()
	if _, _, exists := budgetSnapshot(e, "组合规则"); exists {
		t.Fatal("最后一个实例停止后仍保留规则预算")
	}
	other.Stop()
	globalRejected.Stop()
	if e.active.Load() != 0 {
		t.Fatal("组合停止后全局会话额度泄漏")
	}
}

func TestPreparedReplacementDoesNotResetQuota(t *testing.T) {
	target := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
	e := New(Limits{PerRule: 1})
	old := prepareNamed(t, e, "同一规则", "tcp", 0, target.Addr())
	old.Start()
	c := dialTCP(t, old)
	tcpExchange(t, c)
	replacement := prepareNamed(t, e, "同一规则", "tcp", 0, target.Addr())
	if refs, active, _ := budgetSnapshot(e, "同一规则"); refs != 2 || active != 1 {
		t.Fatalf("预备替换实例丢失旧额度: %d %d", refs, active)
	}
	replacement.Start()
	var one [1]byte
	if _, err := dialTCP(t, replacement).Read(one[:]); err == nil {
		t.Fatal("预备实例重新分配了独立额度")
	}
	tcpExchange(t, c)
	old.Stop()
	if refs, active, _ := budgetSnapshot(e, "同一规则"); refs != 1 || active != 0 {
		t.Fatalf("停止旧实例未保留预备预算引用: %d %d", refs, active)
	}
	tcpExchange(t, dialTCP(t, replacement))
	replacement.Stop()
	if _, _, exists := budgetSnapshot(e, "同一规则"); exists {
		t.Fatal("替换完成后遗留预算")
	}
}

func TestBudgetReferenceCleanupPrepareFailureAndEmptyIDs(t *testing.T) {
	target := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
	e := New(Limits{PerRule: 1})
	r := prepareNamed(t, e, "有效规则", "tcp", 0, target.Addr())
	for _, id := range []string{"有效规则", "失败规则"} {
		_, err := e.Prepare(model.Rule{ID: id, Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: r.tcp.Addr().(*net.TCPAddr).Port})
		if err == nil {
			t.Fatal("占用端口仍成功准备")
		}
	}
	if refs, active, _ := budgetSnapshot(e, "有效规则"); refs != 1 || active != 0 {
		t.Fatalf("准备失败改变预算引用: %d %d", refs, active)
	}
	if _, _, exists := budgetSnapshot(e, "失败规则"); exists {
		t.Fatal("失败准备创建了预算对象")
	}
	r.Stop()
	r.Stop()
	for i := range 64 {
		prepared := prepareNamed(t, e, fmt.Sprint(i), "tcp", 0, target.Addr())
		prepared.Stop()
	}
	e.mu.Lock()
	count := len(e.budgets)
	e.mu.Unlock()
	if count != 0 {
		t.Fatalf("反复准备停止导致预算字典增长: %d", count)
	}
	a := prepareNamed(t, e, "", "tcp", 0, target.Addr())
	b := prepareNamed(t, e, "", "tcp", 0, target.Addr())
	a.Start()
	b.Start()
	tcpExchange(t, dialTCP(t, a))
	tcpExchange(t, dialTCP(t, b))
	if e.active.Load() != 2 {
		t.Fatal("空 ID 没有分配独立预算")
	}
	a.Stop()
	b.Stop()
}

func TestFailedSignalPublishesErrorBeforeConcurrentGroupStop(t *testing.T) {
	for _, protocol := range []string{"tcp", "udp"} {
		t.Run(protocol, func(t *testing.T) {
			tcpTarget := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
			udpTarget := testUDPServer(t, "127.0.0.1")
			e := New(Limits{})
			tcp := prepareNamed(t, e, "故障组", "tcp", 0, tcpTarget.Addr())
			udp := prepareNamed(t, e, "故障组", "udp", tcp.tcp.Addr().(*net.TCPAddr).Port, udpTarget.LocalAddr())
			tcp.Start()
			udp.Start()
			tcpExchange(t, dialTCP(t, tcp))
			if err := udpRoundTrip(udpClient(t, udp), []byte("a")); err != nil {
				t.Fatal(err)
			}
			failed, peer := tcp, udp
			if protocol == "udp" {
				failed, peer = udp, tcp
			}
			notified := make(chan error, 1)
			go func() { <-failed.Failed(); notified <- failed.Error(); peer.Stop(); failed.Stop() }()
			if protocol == "tcp" {
				_ = tcp.tcp.Close()
			} else {
				_ = udp.udp.Close()
			}
			select {
			case err := <-notified:
				if err == nil {
					t.Fatal("故障通道关闭后 Error 仍为空")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("监听故障没有主动通知规则组")
			}
			// 与故障监视者并发停止，必须不重关通道、不自等待且只归还一次预算。
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); tcp.Stop(); udp.Stop() }()
			}
			done := make(chan struct{})
			go func() { wg.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("故障与组停止形成等待环")
			}
			if _, _, exists := budgetSnapshot(e, "故障组"); exists || e.active.Load() != 0 {
				t.Fatal("故障停止后仍持有预算或会话")
			}
			select {
			case <-peer.Failed():
				t.Fatal("正常协同停止被误报为监听故障")
			default:
			}
		})
	}
}

func TestNormalStopAndTargetFailureDoNotSignalListenerFailure(t *testing.T) {
	e := New(Limits{})
	r, err := e.Prepare(model.Rule{Protocol: "tcp", ListenHost: "127.0.0.1", TargetHost: "0.0.0.0", TargetPort: 1})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	t.Cleanup(r.Stop)
	var one [1]byte
	if _, err := dialTCP(t, r).Read(one[:]); err == nil {
		t.Fatal("非法上游地址被接受")
	}
	if r.Error() != nil {
		t.Fatal("上游失败污染监听状态")
	}
	r.Stop()
	select {
	case <-r.Failed():
		t.Fatal("目标失败或正常停止触发了故障通道")
	default:
	}
	// 并发重复故障上报只能发布一次，且保留首次错误。
	prepared, err := e.Prepare(model.Rule{Protocol: "udp", ListenHost: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(prepared.Stop)
	first := errors.New("首次监听故障")
	prepared.failed(first)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); prepared.failed(errors.New("重复故障")) }()
	}
	wg.Wait()
	if !errors.Is(prepared.Error(), first) {
		t.Fatal("重复故障覆盖首次错误")
	}
	prepared.Stop()
}
