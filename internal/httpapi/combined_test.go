package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

func pairedHTTPListeners(t *testing.T) (net.Listener, *net.UDPConn, int) {
	t.Helper()
	for range 16 {
		tcp, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := tcp.Addr().(*net.TCPAddr).Port
		udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err == nil {
			return tcp, udp, port
		}
		_ = tcp.Close()
	}
	t.Fatal("没有找到 TCP/UDP 同时可用端口")
	return nil, nil, 0
}

func pairedHTTPTarget(t *testing.T) int {
	t.Helper()
	tcp, udp, port := pairedHTTPListeners(t)
	tcpDone, udpDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(tcpDone)
		for {
			c, err := tcp.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _ = c.SetDeadline(time.Now().Add(4 * time.Second)); _, _ = io.Copy(c, c) }()
		}
	}()
	go func() {
		defer close(udpDone)
		buf := make([]byte, 65535)
		for {
			n, peer, err := udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = udp.WriteToUDP(buf[:n], peer)
		}
	}()
	t.Cleanup(func() { _ = tcp.Close(); _ = udp.Close(); <-tcpDone; <-udpDone })
	return port
}

func httpCombinedPort(t *testing.T) int {
	t.Helper()
	tcp, udp, port := pairedHTTPListeners(t)
	_ = tcp.Close()
	_ = udp.Close()
	return port
}

func combinedHTTPClient(t *testing.T, port int) *net.TCPConn {
	t.Helper()
	c, err := net.DialTCP("tcp4", nil, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func httpTCPExchange(t *testing.T, c *net.TCPConn, text string) {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(text))
	if _, err := io.ReadFull(c, got); err != nil || string(got) != text {
		t.Fatalf("HTTP 配置后的 TCP 未正确转发：%q %v", got, err)
	}
}

func verifyHTTPCombinedForwarding(t *testing.T, port int) {
	t.Helper()
	tcp := combinedHTTPClient(t, port)
	payload := []byte("HTTP 组合规则 TCP")
	_, _ = tcp.Write(payload)
	_ = tcp.CloseWrite()
	got, err := io.ReadAll(tcp)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("组合 TCP 半关闭后响应错误：%q %v", got, err)
	}
	udp, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	_ = udp.SetDeadline(time.Now().Add(time.Second))
	for _, payload := range [][]byte{[]byte("HTTP 组合规则 UDP"), {}} {
		if _, err := udp.Write(payload); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 65535)
		n, err := udp.Read(buf)
		if err != nil || !bytes.Equal(payload, buf[:n]) {
			t.Fatalf("组合 UDP 未保留数据报：%q %v", buf[:n], err)
		}
	}
}

func expectHTTPCombinedPortFree(t *testing.T, port int) {
	t.Helper()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	tcp, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("HTTP 操作完成后 TCP 监听未释放：%v", err)
	}
	defer tcp.Close()
	udp, err := net.ListenPacket("udp4", address)
	if err != nil {
		t.Fatalf("HTTP 操作完成后 UDP 监听未释放：%v", err)
	}
	_ = udp.Close()
}

func TestHTTPCombinedRuleOneRequestFullLifecycle(t *testing.T) {
	f := newAPI(t, "", false)
	f.login(t)
	target, listen := pairedHTTPTarget(t), httpCombinedPort(t)
	enabled := true
	in := model.Input{Name: "统一业务入口", Protocol: model.ProtocolBoth, ListenHost: "127.0.0.1", ListenPort: listen, TargetHost: "127.0.0.1", TargetPort: target, Enabled: &enabled}
	created := f.request(t, "POST", "/api/rules", inputJSON(t, in), nil, true)
	expectStatus(t, created, 201)
	rule := responseRule(t, created)
	if rule.Protocol != model.ProtocolBoth || rule.Revision != 1 || rule.Status != "running" {
		t.Fatalf("HTTP 创建没有返回一个组合规则：%s", created.body)
	}
	if list := ruleList(t, f); len(list) != 1 || list[0].ID != rule.ID {
		t.Fatalf("一次创建被拆为多条规则：%+v", list)
	}
	rows, err := f.db.List(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Protocol != model.ProtocolBoth {
		t.Fatalf("一次请求没有持久化为单条组合：%+v %v", rows, err)
	}
	verifyHTTPCombinedForwarding(t, listen)
	path := "/api/rules/" + rule.ID
	kept := combinedHTTPClient(t, listen)
	httpTCPExchange(t, kept, "重命名前")
	in.Name = "修改后的统一入口"
	edited := f.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"1"`}, true)
	expectStatus(t, edited, 200)
	rule = responseRule(t, edited)
	if rule.Revision != 2 || rule.Protocol != model.ProtocolBoth || rule.Name != in.Name {
		t.Fatalf("组合编辑响应不正确：%s", edited.body)
	}
	httpTCPExchange(t, kept, "重命名后连接保留")
	expectCode(t, f.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"1"`}, true), 412, "revision_mismatch")
	enabled = false
	disabled := f.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"2"`}, true)
	expectStatus(t, disabled, 200)
	if r := responseRule(t, disabled); r.Enabled || r.Revision != 3 || r.Status != "disabled" || r.Protocol != model.ProtocolBoth {
		t.Fatalf("整体停用响应不正确：%s", disabled.body)
	}
	_ = kept.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	_, err = kept.Read(b[:])
	if err == nil {
		t.Fatal("整体停用没有断开 TCP 会话")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatal("整体停用后旧 TCP 仅超时，没有关闭")
	}
	expectHTTPCombinedPortFree(t, listen)
	if len(ruleList(t, f)) != 1 {
		t.Fatal("停用组合后规则记录丢失")
	}
	enabled = true
	expectCode(t, f.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"2"`}, true), 412, "revision_mismatch")
	reenabled := f.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"3"`}, true)
	expectStatus(t, reenabled, 200)
	if r := responseRule(t, reenabled); r.Status != "running" || r.Revision != 4 {
		t.Fatalf("整体启用响应不正确：%s", reenabled.body)
	}
	verifyHTTPCombinedForwarding(t, listen)
	in.Protocol = model.ProtocolTCP
	single := f.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"4"`}, true)
	expectStatus(t, single, 200)
	if r := responseRule(t, single); r.Protocol != model.ProtocolTCP || r.Revision != 5 {
		t.Fatalf("组合转单协议响应错误：%s", single.body)
	}
	udpProbe, err := net.ListenPacket("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(listen)))
	if err != nil {
		t.Fatalf("组合改为 TCP 后 UDP 监听残留：%v", err)
	}
	_ = udpProbe.Close()
	in.Protocol = model.ProtocolBoth
	combined := f.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"5"`}, true)
	expectStatus(t, combined, 200)
	if r := responseRule(t, combined); r.ID != rule.ID || r.Protocol != model.ProtocolBoth || r.Revision != 6 {
		t.Fatalf("单协议恢复组合身份错误：%s", combined.body)
	}
	verifyHTTPCombinedForwarding(t, listen)
	expectCode(t, f.request(t, "DELETE", path, "", map[string]string{"If-Match": `"5"`}, true), 412, "revision_mismatch")
	expectStatus(t, f.request(t, "DELETE", path, "", map[string]string{"If-Match": `"6"`}, true), 204)
	if len(ruleList(t, f)) != 0 {
		t.Fatal("删除组合后 API 列表仍有记录")
	}
	rows, err = f.db.List(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("删除组合后数据库仍有记录")
	}
	expectHTTPCombinedPortFree(t, listen)
}

func TestHTTPCombinedCreationRejectsOverlappingSingleProtocol(t *testing.T) {
	for _, protocol := range model.Transports(model.ProtocolBoth) {
		t.Run(protocol, func(t *testing.T) {
			f := newAPI(t, "", false)
			f.login(t)
			disabled := false
			in := model.Input{Name: "原规则", Protocol: protocol, ListenHost: "0.0.0.0", ListenPort: httpCombinedPort(t), TargetHost: "127.0.0.1", TargetPort: 1, Enabled: &disabled}
			expectStatus(t, f.request(t, "POST", "/api/rules", inputJSON(t, in), nil, true), 201)
			in.Protocol = model.ProtocolBoth
			in.ListenHost = "127.0.0.1"
			conflict := f.request(t, "POST", "/api/rules", inputJSON(t, in), nil, true)
			expectCode(t, conflict, 409, "rule_conflict")
			if !bytes.Contains(conflict.body, bytes.ToUpper([]byte(protocol))) {
				t.Fatalf("API 冲突未指明传输协议：%s", conflict.body)
			}
			if len(ruleList(t, f)) != 1 {
				t.Fatal("组合冲突请求留下部分记录")
			}
		})
	}
}
