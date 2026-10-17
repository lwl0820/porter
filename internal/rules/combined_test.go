package rules_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

type combinedTarget struct {
	port    int
	sources chan int
}

// TCP 与 UDP 回显服务绑定同一数字端口，防止测试用两个目标掩盖组合契约问题。
func pairedTarget(t *testing.T) combinedTarget {
	t.Helper()
	port := echo(t)
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	sources := make(chan int, 128)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		for {
			n, peer, err := udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			select {
			case sources <- peer.Port:
			default:
			}
			_, _ = udp.WriteToUDP(buf[:n], peer)
		}
	}()
	t.Cleanup(func() { _ = udp.Close(); <-done })
	return combinedTarget{port: port, sources: sources}
}

func combinedPort(t *testing.T) int {
	t.Helper()
	for range 16 {
		tcp, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := tcp.Addr().(*net.TCPAddr).Port
		udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		_ = tcp.Close()
		if err == nil {
			_ = udp.Close()
			return port
		}
	}
	t.Fatal("找不到 TCP/UDP 同时空闲的端口")
	return 0
}

func bothInput(port, target int) model.Input {
	in := input(port, target)
	in.Protocol = model.ProtocolBoth
	return in
}

func ruleTCP(t *testing.T, port int) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func ruleUDP(t *testing.T, port int) *net.UDPConn {
	t.Helper()
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func combinedUDPExchange(t *testing.T, c *net.UDPConn, target combinedTarget, text string) int {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	payload := []byte(text)
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 65535)
	n, err := c.Read(buf)
	if err != nil || !bytes.Equal(buf[:n], payload) {
		t.Fatalf("UDP 收发不匹配：%q %v", buf[:n], err)
	}
	select {
	case port := <-target.sources:
		return port
	case <-time.After(time.Second):
		t.Fatal("目标未观察到 UDP 会话")
		return 0
	}
}

func tcpClosed(t *testing.T, c net.Conn) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	n, err := c.Read(b[:])
	if n != 0 || err == nil {
		t.Fatal("协议改变后旧 TCP 会话仍存活")
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		t.Fatal("旧 TCP 会话没有被关闭，只是读取超时")
	}
}

func occupy(t *testing.T, protocol string, port int) io.Closer {
	t.Helper()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	var closer io.Closer
	var err error
	if protocol == model.ProtocolTCP {
		closer, err = net.Listen("tcp4", address)
	} else {
		closer, err = net.ListenPacket("udp4", address)
	}
	if err != nil {
		t.Fatalf("%s 端口 %d 未释放：%v", protocol, port, err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	return closer
}

func protocolsFree(t *testing.T, port int) {
	t.Helper()
	for _, protocol := range model.Transports(model.ProtocolBoth) {
		c := occupy(t, protocol, port)
		_ = c.Close()
	}
}

func TestCombinedRealForwardingAndAllProtocolConversions(t *testing.T) {
	protocols := []string{model.ProtocolTCP, model.ProtocolUDP, model.ProtocolBoth}
	for _, from := range protocols {
		for _, to := range protocols {
			if from == to {
				continue
			}
			t.Run(from+"到"+to, func(t *testing.T) {
				s, repo := setup(t)
				target := pairedTarget(t)
				port := combinedPort(t)
				in := input(port, target.port)
				in.Protocol = from
				original, err := s.Save(context.Background(), "", 0, in)
				if err != nil {
					t.Fatal(err)
				}
				var oldTCP net.Conn
				var oldUDP *net.UDPConn
				oldUpstreamPort := 0
				if model.HasTransport(from, model.ProtocolTCP) {
					oldTCP = ruleTCP(t, port)
					exchange(t, oldTCP, "修改前 TCP")
				}
				if model.HasTransport(from, model.ProtocolUDP) {
					oldUDP = ruleUDP(t, port)
					oldUpstreamPort = combinedUDPExchange(t, oldUDP, target, "修改前 UDP")
				}
				in.Name = "仅修改名称"
				renamed, err := s.Save(context.Background(), original.ID, original.Revision, in)
				if err != nil {
					t.Fatal(err)
				}
				if oldTCP != nil {
					exchange(t, oldTCP, "名称保留连接")
				}
				if oldUDP != nil && combinedUDPExchange(t, oldUDP, target, "名称保留 UDP 会话") != oldUpstreamPort {
					t.Fatal("只修改名称重建了 UDP 上游会话")
				}
				in.Protocol = to
				changed, err := s.Save(context.Background(), original.ID, renamed.Revision, in)
				if err != nil {
					t.Fatal(err)
				}
				if oldTCP != nil {
					tcpClosed(t, oldTCP)
				}
				if oldUDP != nil {
					probe := occupy(t, model.ProtocolUDP, oldUpstreamPort)
					_ = probe.Close()
				}
				if model.HasTransport(to, model.ProtocolTCP) {
					exchange(t, ruleTCP(t, port), "转换后 TCP")
				} else {
					probe := occupy(t, model.ProtocolTCP, port)
					_ = probe.Close()
				}
				if model.HasTransport(to, model.ProtocolUDP) {
					combinedUDPExchange(t, ruleUDP(t, port), target, "转换后 UDP")
				} else {
					probe := occupy(t, model.ProtocolUDP, port)
					_ = probe.Close()
				}
				if changed.ID != original.ID || changed.Revision != 3 || changed.Status != "running" || !changed.CreatedAt.Equal(original.CreatedAt) {
					t.Fatalf("协议转换改变了规则身份或状态：%+v", changed)
				}
				list := s.List()
				rows, err := repo.List(context.Background())
				if err != nil || len(list) != 1 || len(rows) != 1 || rows[0].Protocol != to || rows[0].ID != original.ID {
					t.Fatalf("组合被拆成多条或未正确持久化：%+v %+v %v", list, rows, err)
				}
			})
		}
	}
}

func TestCombinedExternalOccupancyCreationIsAtomic(t *testing.T) {
	for _, protocol := range model.Transports(model.ProtocolBoth) {
		t.Run(protocol, func(t *testing.T) {
			s, repo := setup(t)
			target := pairedTarget(t)
			port := combinedPort(t)
			block := occupy(t, protocol, port)
			_, err := s.Save(context.Background(), "", 0, bothInput(port, target.port))
			status(t, err, 409)
			if !strings.Contains(err.Error(), strings.ToUpper(protocol)) {
				t.Fatalf("监听冲突未指出具体协议：%v", err)
			}
			rows, err := repo.List(context.Background())
			if err != nil || len(rows) != 0 || len(s.List()) != 0 {
				t.Fatal("部分准备成功后留下规则配置")
			}
			other := model.ProtocolTCP
			if protocol == model.ProtocolTCP {
				other = model.ProtocolUDP
			}
			probe := occupy(t, other, port)
			_ = probe.Close()
			_ = block.Close()
			protocolsFree(t, port)
		})
	}
}

func TestCombinedCreateCommitFailureReleasesBothListeners(t *testing.T) {
	s, repo := setup(t)
	target := pairedTarget(t)
	port := combinedPort(t)
	repo.fail = true
	_, err := s.Save(context.Background(), "", 0, bothInput(port, target.port))
	status(t, err, 500)
	if len(s.List()) != 0 {
		t.Fatal("提交失败留下组合规则")
	}
	rows, err := repo.List(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("提交失败写入了组合规则")
	}
	protocolsFree(t, port)
}

func TestTCPUpgradeFailurePreservesOriginalConnection(t *testing.T) {
	for _, failure := range []string{"UDP 被占用", "数据库提交失败"} {
		t.Run(failure, func(t *testing.T) {
			s, repo := setup(t)
			target := pairedTarget(t)
			port := combinedPort(t)
			in := input(port, target.port)
			original, err := s.Save(context.Background(), "", 0, in)
			if err != nil {
				t.Fatal(err)
			}
			c := ruleTCP(t, port)
			exchange(t, c, "升级前")
			var block io.Closer
			if failure == "UDP 被占用" {
				block = occupy(t, model.ProtocolUDP, port)
			} else {
				repo.fail = true
			}
			in.Protocol = model.ProtocolBoth
			_, err = s.Save(context.Background(), original.ID, original.Revision, in)
			if block != nil {
				status(t, err, 409)
				if !strings.Contains(err.Error(), "UDP") {
					t.Fatal("升级冲突未指出 UDP")
				}
			} else {
				status(t, err, 500)
				probe := occupy(t, model.ProtocolUDP, port)
				_ = probe.Close()
			}
			exchange(t, c, "升级失败后仍然转发")
			got := s.List()[0]
			rows, err := repo.List(context.Background())
			if err != nil || got.Protocol != model.ProtocolTCP || got.Revision != original.Revision || len(rows) != 1 || rows[0].Protocol != model.ProtocolTCP || rows[0].Revision != original.Revision {
				t.Fatalf("失败升级改变了原配置：%+v %+v %v", got, rows, err)
			}
		})
	}
}

func TestCombinedUpdateAndDeleteCommitFailurePreserveBothSessions(t *testing.T) {
	s, repo := setup(t)
	target := pairedTarget(t)
	otherTarget := pairedTarget(t)
	port := combinedPort(t)
	in := bothInput(port, target.port)
	original, err := s.Save(context.Background(), "", 0, in)
	if err != nil {
		t.Fatal(err)
	}
	c, u := ruleTCP(t, port), ruleUDP(t, port)
	exchange(t, c, "初始 TCP")
	upstreamPort := combinedUDPExchange(t, u, target, "初始 UDP")
	for _, change := range []string{"目标", "监听地址", "协议"} {
		t.Run(change, func(t *testing.T) {
			candidate := in
			switch change {
			case "目标":
				candidate.TargetPort = otherTarget.port
			case "监听地址":
				candidate.ListenPort = combinedPort(t)
			case "协议":
				candidate.Protocol = model.ProtocolTCP
			}
			repo.fail = true
			_, err := s.Save(context.Background(), original.ID, original.Revision, candidate)
			status(t, err, 500)
			exchange(t, c, "失败后 TCP")
			if combinedUDPExchange(t, u, target, "失败后 UDP") != upstreamPort {
				t.Fatal("提交失败重建了旧 UDP 会话")
			}
			if candidate.ListenPort != port {
				protocolsFree(t, candidate.ListenPort)
			}
			got := s.List()[0]
			if got.Protocol != model.ProtocolBoth || got.Revision != 1 || got.ListenPort != port || got.TargetPort != target.port {
				t.Fatalf("提交失败改变内存配置：%+v", got)
			}
		})
	}
	repo.fail = true
	status(t, s.Delete(context.Background(), original.ID, original.Revision), 500)
	exchange(t, c, "删除失败仍可转发")
	if combinedUDPExchange(t, u, target, "删除失败 UDP") != upstreamPort {
		t.Fatal("删除失败结束了 UDP 会话")
	}
	rows, err := repo.List(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Protocol != model.ProtocolBoth || rows[0].Revision != 1 {
		t.Fatal("失败操作改变数据库")
	}
	repo.fail = false
	if err := s.Delete(context.Background(), original.ID, original.Revision); err != nil {
		t.Fatal(err)
	}
	tcpClosed(t, c)
	protocolsFree(t, port)
}

func TestCombinedTargetChangeEndsBothOldSessions(t *testing.T) {
	s, _ := setup(t)
	oldTarget, newTarget := pairedTarget(t), pairedTarget(t)
	port := combinedPort(t)
	in := bothInput(port, oldTarget.port)
	rule, err := s.Save(context.Background(), "", 0, in)
	if err != nil {
		t.Fatal(err)
	}
	c, u := ruleTCP(t, port), ruleUDP(t, port)
	exchange(t, c, "原目标 TCP")
	oldUpstreamPort := combinedUDPExchange(t, u, oldTarget, "原目标 UDP")
	in.TargetPort = newTarget.port
	updated, err := s.Save(context.Background(), rule.ID, rule.Revision, in)
	if err != nil {
		t.Fatal(err)
	}
	tcpClosed(t, c)
	probe := occupy(t, model.ProtocolUDP, oldUpstreamPort)
	defer probe.Close()
	exchange(t, ruleTCP(t, port), "新目标 TCP")
	combinedUDPExchange(t, u, newTarget, "新目标 UDP")
	if updated.Protocol != model.ProtocolBoth || updated.Revision != 2 || updated.TargetPort != newTarget.port || updated.Status != "running" {
		t.Fatalf("组合目标更新未整体生效：%+v", updated)
	}
}

func TestCombinedRestoreFailureReleasesPeerAndRetryRecovers(t *testing.T) {
	for _, blocked := range model.Transports(model.ProtocolBoth) {
		t.Run(blocked, func(t *testing.T) {
			path := filepath.Join(testutil.TempDir(t), "restore-combined.db")
			db, err := store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			engine := forward.New(forward.Limits{})
			old, err := rules.New(context.Background(), db, engine, "127.0.0.1:8080")
			if err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { old.Close(); _ = db.Close() })
			target := pairedTarget(t)
			port := combinedPort(t)
			combined, err := old.Save(context.Background(), "", 0, bothInput(port, target.port))
			if err != nil {
				t.Fatal(err)
			}
			separate, err := old.Save(context.Background(), "", 0, input(combinedPort(t), target.port))
			if err != nil {
				t.Fatal(err)
			}
			old.Close()
			_ = db.Close()
			block := occupy(t, blocked, port)
			db, err = store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := rules.New(context.Background(), db, engine, "127.0.0.1:8080")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restored.Close() })
			for _, r := range restored.List() {
				if r.ID == combined.ID && (r.Status != "error" || !strings.Contains(r.LastError, strings.ToUpper(blocked))) {
					t.Fatalf("半恢复的组合未整体报错：%+v", r)
				}
				if r.ID == separate.ID && r.Status != "running" {
					t.Fatal("组合失败阻断其他规则")
				}
			}
			peer := model.ProtocolTCP
			if blocked == model.ProtocolTCP {
				peer = model.ProtocolUDP
			}
			probe := occupy(t, peer, port)
			_ = probe.Close()
			exchange(t, ruleTCP(t, separate.ListenPort), "独立规则仍运行")
			_ = block.Close()
			retried, err := restored.Retry(combined.ID, combined.Revision)
			if err != nil || retried.Status != "running" || retried.Protocol != model.ProtocolBoth {
				t.Fatalf("整体重试失败：%+v %v", retried, err)
			}
			exchange(t, ruleTCP(t, port), "组合恢复 TCP")
			combinedUDPExchange(t, ruleUDP(t, port), target, "组合恢复 UDP")
		})
	}
}

func TestCombinedCrossProtocolWildcardAndAdminConflicts(t *testing.T) {
	for _, existing := range []string{model.ProtocolTCP, model.ProtocolUDP, model.ProtocolBoth} {
		for _, candidate := range []string{model.ProtocolTCP, model.ProtocolUDP, model.ProtocolBoth} {
			common := model.CommonTransport(existing, candidate)
			if common == "" {
				continue
			}
			t.Run(existing+"与"+candidate, func(t *testing.T) {
				s, _ := setup(t)
				disabled := false
				in := input(combinedPort(t), 1)
				in.Protocol = existing
				in.ListenHost = "0.0.0.0"
				in.Enabled = &disabled
				if _, err := s.Save(context.Background(), "", 0, in); err != nil {
					t.Fatal(err)
				}
				in.Protocol = candidate
				in.ListenHost = "127.0.0.1"
				_, err := s.Save(context.Background(), "", 0, in)
				status(t, err, 409)
				if !strings.Contains(err.Error(), strings.ToUpper(common)) {
					t.Fatalf("交叉或通配冲突未指明 %s：%v", common, err)
				}
			})
		}
	}
	s, _ := setup(t)
	_, err := s.Save(context.Background(), "", 0, bothInput(8080, 1))
	status(t, err, 409)
	if !strings.Contains(err.Error(), "管理") || !strings.Contains(err.Error(), "TCP") {
		t.Fatalf("组合管理端口冲突文案不明确：%v", err)
	}
}
