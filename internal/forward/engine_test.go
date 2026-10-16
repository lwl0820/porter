package forward

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

func testTCPServer(t *testing.T, host string, handle func(*net.TCPConn)) *net.TCPListener {
	t.Helper()
	network := "tcp4"
	if host == "::1" {
		network = "tcp6"
	}
	ln, err := net.ListenTCP(network, &net.TCPAddr{IP: net.ParseIP(host)})
	if err != nil {
		if host == "::1" {
			t.Skipf("当前环境没有 IPv6 回环: %v", err)
		}
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.AcceptTCP()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				handle(c)
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close(); <-done; wg.Wait() })
	return ln
}

func testRuntime(t *testing.T, e *Engine, protocol, host string, target net.Addr) *Runtime {
	t.Helper()
	targetHost, targetPort, err := net.SplitHostPort(target.String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(targetPort)
	r, err := e.Prepare(model.Rule{Protocol: protocol, ListenHost: host, TargetHost: targetHost, TargetPort: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)
	r.Start()
	return r
}

func dialTCP(t *testing.T, r *Runtime) *net.TCPConn {
	t.Helper()
	c, err := net.DialTCP("tcp", nil, r.tcp.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func await(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待资源状态超时")
}

func TestTCPHalfCloseIPv4IPv6(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			payload := bytes.Repeat([]byte("响应"), 40000)
			server := testTCPServer(t, host, func(c *net.TCPConn) {
				data, _ := io.ReadAll(c)
				if string(data) == "请求" {
					_, _ = c.Write(payload)
				}
			})
			e := New(Limits{})
			r := testRuntime(t, e, "tcp", host, server.Addr())
			c := dialTCP(t, r)
			_, _ = c.Write([]byte("请求"))
			if err := c.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(c)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("半关闭后响应不完整: 长度 %d, 错误 %v", len(got), err)
			}
			await(t, func() bool { return e.active.Load() == 0 })
		})
	}
}

func TestPrepareStopAndConcurrentStopReleasePort(t *testing.T) {
	e := New(Limits{})
	r, err := e.Prepare(model.Rule{Protocol: "tcp", ListenHost: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	address := r.tcp.Addr().String()
	if _, err := net.Listen("tcp4", address); err == nil {
		t.Fatal("预备监听没有占用端口")
	}
	r.Stop()
	r.Start()
	ln, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("未启动监听停止后端口未释放: %v", err)
	}
	_ = ln.Close()
	server := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
	r = testRuntime(t, e, "tcp", "127.0.0.1", server.Addr())
	c := dialTCP(t, r)
	_, _ = c.Write([]byte("a"))
	var one [1]byte
	if _, err := io.ReadFull(c, one[:]); err != nil {
		t.Fatal(err)
	}
	address = r.tcp.Addr().String()
	var stops sync.WaitGroup
	for range 16 {
		stops.Add(1)
		go func() { defer stops.Done(); r.Stop() }()
	}
	stops.Wait()
	if e.active.Load() != 0 {
		t.Fatal("停止后仍占用会话额度")
	}
	if _, err := c.Read(one[:]); err == nil {
		t.Fatal("停止后客户端仍可读写")
	}
	ln, err = net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("停止后端口未释放: %v", err)
	}
	_ = ln.Close()
}

func TestUpdateClosesOldTCPAndChangesTarget(t *testing.T) {
	first := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
	second := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = c.Write([]byte("新目标")) })
	e := New(Limits{})
	r := testRuntime(t, e, "tcp", "127.0.0.1", first.Addr())
	c := dialTCP(t, r)
	_, _ = c.Write([]byte("a"))
	var one [1]byte
	if _, err := io.ReadFull(c, one[:]); err != nil {
		t.Fatal(err)
	}
	rule := r.current.rule
	rule.TargetPort = second.Addr().(*net.TCPAddr).Port
	r.Update(rule)
	if e.active.Load() != 0 {
		t.Fatal("更新后旧会话仍占用额度")
	}
	if _, err := c.Read(one[:]); err == nil {
		t.Fatal("旧 TCP 连接未关闭")
	}
	got, err := io.ReadAll(dialTCP(t, r))
	if err != nil || string(got) != "新目标" {
		t.Fatalf("更新目标失败: %q %v", got, err)
	}
}

func testUDPServer(t *testing.T, host string) *net.UDPConn {
	t.Helper()
	network := "udp4"
	if host == "::1" {
		network = "udp6"
	}
	c, err := net.ListenUDP(network, &net.UDPAddr{IP: net.ParseIP(host)})
	if err != nil {
		if host == "::1" {
			t.Skipf("当前环境没有 IPv6 UDP: %v", err)
		}
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 65535)
		for {
			n, peer, err := c.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			_, _ = c.WriteToUDP(buffer[:n], peer)
		}
	}()
	t.Cleanup(func() { _ = c.Close(); <-done })
	return c
}

func udpClient(t *testing.T, r *Runtime) *net.UDPConn {
	t.Helper()
	c, err := net.DialUDP("udp", nil, r.udp.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func udpRoundTrip(c *net.UDPConn, payload []byte) error {
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write(payload); err != nil {
		return err
	}
	buffer := make([]byte, 65535)
	n, err := c.Read(buffer)
	if err != nil {
		return err
	}
	if !bytes.Equal(payload, buffer[:n]) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func TestUDPMultiClientZeroLengthExpiryAndIPv6(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			server := testUDPServer(t, host)
			e := New(Limits{UDPIdle: 150 * time.Millisecond})
			r := testRuntime(t, e, "udp", host, server.LocalAddr())
			a, b := udpClient(t, r), udpClient(t, r)
			var workers sync.WaitGroup
			for i, c := range []*net.UDPConn{a, b} {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for j := range 20 {
						payload := bytes.Repeat([]byte{byte(i), byte(j)}, 2000)
						if err := udpRoundTrip(c, payload); err != nil {
							t.Errorf("UDP 客户端 %d 串包或收发失败: %v", i, err)
							return
						}
					}
				}()
			}
			workers.Wait()
			if err := udpRoundTrip(a, []byte{}); err != nil {
				t.Fatalf("零长度数据报未保留: %v", err)
			}
			if err := udpRoundTrip(a, bytes.Repeat([]byte{7}, 60000)); err != nil {
				t.Fatalf("大数据报未完整保留: %v", err)
			}
			if e.active.Load() != 2 {
				t.Fatalf("每客户端未建立独立会话: %d", e.active.Load())
			}
			await(t, func() bool { return e.active.Load() == 0 })
			if err := udpRoundTrip(a, []byte("过期后重建")); err != nil {
				t.Fatal(err)
			}
			r.Stop()
			if e.active.Load() != 0 {
				t.Fatal("UDP 停止后仍占用会话额度")
			}
			ln, err := net.ListenUDP("udp", r.udp.LocalAddr().(*net.UDPAddr))
			if err != nil {
				t.Fatalf("UDP 停止后端口未释放: %v", err)
			}
			_ = ln.Close()
		})
	}
}

func TestSessionLimitsAcrossTCPAndUDP(t *testing.T) {
	server := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
	udpServer := testUDPServer(t, "127.0.0.1")
	e := New(Limits{Global: 2, PerRule: 1})
	r := testRuntime(t, e, "tcp", "127.0.0.1", server.Addr())
	u := testRuntime(t, e, "udp", "127.0.0.1", udpServer.LocalAddr())
	c := dialTCP(t, r)
	_, _ = c.Write([]byte("a"))
	var one [1]byte
	if _, err := io.ReadFull(c, one[:]); err != nil {
		t.Fatal(err)
	}
	second := dialTCP(t, r)
	if _, err := second.Read(one[:]); err == nil {
		t.Fatal("单规则额度没有拒绝额外连接")
	}
	if err := udpRoundTrip(udpClient(t, u), []byte("a")); err != nil {
		t.Fatal(err)
	}
	other := testRuntime(t, e, "tcp", "127.0.0.1", server.Addr())
	third := dialTCP(t, other)
	if _, err := third.Read(one[:]); err == nil {
		t.Fatal("全局额度没有覆盖 TCP 和 UDP 合计")
	}
	u.Stop()
	fourth := dialTCP(t, other)
	_, _ = fourth.Write([]byte("b"))
	if _, err := io.ReadFull(fourth, one[:]); err != nil || one[0] != 'b' {
		t.Fatalf("额度释放后未恢复: %v", err)
	}
}

func TestResolvedDirectLoopRejected(t *testing.T) {
	e := New(Limits{Global: 4})
	r, err := e.Prepare(model.Rule{Protocol: "tcp", ListenHost: "127.0.0.1", TargetHost: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	r.current.rule.ListenPort = r.tcp.Addr().(*net.TCPAddr).Port
	r.current.rule.TargetPort = r.current.rule.ListenPort
	r.Start()
	t.Cleanup(r.Stop)
	c := dialTCP(t, r)
	var one [1]byte
	if _, err := c.Read(one[:]); err == nil {
		t.Fatal("域名解析后的自身回环未拒绝")
	}
	await(t, func() bool { return e.active.Load() == 0 })
	if r.Error() != nil {
		t.Fatalf("目标失败不应被标记为监听失败: %v", r.Error())
	}
}

func TestUpdateCancelsPendingDNSAndLateSessions(t *testing.T) {
	original := net.DefaultResolver
	entered := make(chan struct{}, 8)
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	t.Cleanup(func() { net.DefaultResolver = original })
	e := New(Limits{DialTimeout: time.Minute})
	r, err := e.Prepare(model.Rule{Protocol: "tcp", ListenHost: "127.0.0.1", TargetHost: "pending.invalid", TargetPort: 1234})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	t.Cleanup(r.Stop)
	_ = dialTCP(t, r)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("没有开始 DNS 查询")
	}
	rule := r.current.rule
	rule.TargetHost = "127.0.0.1"
	done := make(chan struct{})
	go func() { r.Update(rule); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("更新未取消旧目标拨号")
	}
	if e.active.Load() != 0 {
		t.Fatal("旧拨号在更新后仍占用额度")
	}
}

func TestUDPUpdateClosesOldSessionAndRetainsListener(t *testing.T) {
	first := testUDPServer(t, "127.0.0.1")
	second, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 65535)
		for {
			_, peer, err := second.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			_, _ = second.WriteToUDP([]byte("新目标"), peer)
		}
	}()
	t.Cleanup(func() { _ = second.Close(); <-done })
	e := New(Limits{})
	r := testRuntime(t, e, "udp", "127.0.0.1", first.LocalAddr())
	c := udpClient(t, r)
	if err := udpRoundTrip(c, []byte("原目标")); err != nil {
		t.Fatal(err)
	}
	address := r.udp.LocalAddr().String()
	rule := r.current.rule
	rule.TargetPort = second.LocalAddr().(*net.UDPAddr).Port
	r.Update(rule)
	if e.active.Load() != 0 {
		t.Fatal("UDP 更新后旧会话仍占用额度")
	}
	if r.udp.LocalAddr().String() != address {
		t.Fatal("更新目标改变了监听器")
	}
	_ = c.SetDeadline(time.Now().Add(time.Second))
	_, _ = c.Write([]byte("请求"))
	buffer := make([]byte, 128)
	n, err := c.Read(buffer)
	if err != nil || string(buffer[:n]) != "新目标" {
		t.Fatalf("UDP 会话未切换目标: %q %v", buffer[:n], err)
	}
}

func TestListenerFailureClosesSessionsAndReleasesPort(t *testing.T) {
	server := testTCPServer(t, "127.0.0.1", func(c *net.TCPConn) { _, _ = io.Copy(c, c) })
	e := New(Limits{})
	r := testRuntime(t, e, "tcp", "127.0.0.1", server.Addr())
	c := dialTCP(t, r)
	_, _ = c.Write([]byte("a"))
	var one [1]byte
	if _, err := io.ReadFull(c, one[:]); err != nil {
		t.Fatal(err)
	}
	address := r.tcp.Addr().String()
	_ = r.tcp.Close()
	await(t, func() bool { return r.Error() != nil && e.active.Load() == 0 })
	ln, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("故障监听器未释放端口: %v", err)
	}
	_ = ln.Close()
	r.Stop()
}

func TestUDPNewSessionsRejectedAtLimitAndRecover(t *testing.T) {
	server := testUDPServer(t, "127.0.0.1")
	e := New(Limits{Global: 1, PerRule: 1, UDPIdle: 150 * time.Millisecond})
	r := testRuntime(t, e, "udp", "127.0.0.1", server.LocalAddr())
	a, b := udpClient(t, r), udpClient(t, r)
	if err := udpRoundTrip(a, []byte("a")); err != nil {
		t.Fatal(err)
	}
	_ = b.SetDeadline(time.Now().Add(40 * time.Millisecond))
	_, _ = b.Write([]byte("b"))
	var buffer [1]byte
	if _, err := b.Read(buffer[:]); err == nil {
		t.Fatal("达到限额后仍建立 UDP 会话")
	}
	await(t, func() bool { return e.active.Load() == 0 })
	if err := udpRoundTrip(b, []byte("b")); err != nil {
		t.Fatalf("旧会话过期后 UDP 配额未恢复: %v", err)
	}
}
