package integration

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
)

// 仅使用本地真实 socket，验证外部调用约定与普通代理的来源隔离。
func TestTCPForwardingThroughPublicRuntime(t *testing.T) {
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	done := make(chan error, 1)
	go func() {
		c, err := target.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		data, err := io.ReadAll(c)
		if err == nil {
			_, err = c.Write(append([]byte("返回:"), data...))
		}
		done <- err
	}()
	port := availableForwardPort(t, "tcp")
	r, err := forward.New(forward.Limits{}).Prepare(model.Rule{Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: port, TargetHost: "localhost", TargetPort: target.Addr().(*net.TCPAddr).Port})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	r.Start()
	c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = c.Write([]byte("端口转发"))
	_ = c.(*net.TCPConn).CloseWrite()
	got, err := io.ReadAll(c)
	if err != nil || string(got) != "返回:端口转发" {
		t.Fatalf("TCP 真实转发失败: %q %v", got, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUDPForwardingThroughPublicRuntime(t *testing.T) {
	target, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	done := make(chan error, 1)
	source := make(chan int, 1)
	go func() {
		_ = target.SetDeadline(time.Now().Add(3 * time.Second))
		buffer := make([]byte, 65535)
		n, peer, err := target.ReadFromUDP(buffer)
		if err == nil {
			source <- peer.Port
			_, err = target.WriteToUDP(buffer[:n], peer)
		}
		done <- err
	}()
	port := availableForwardPort(t, "udp")
	r, err := forward.New(forward.Limits{}).Prepare(model.Rule{Protocol: "udp", ListenHost: "127.0.0.1", ListenPort: port, TargetHost: "127.0.0.1", TargetPort: target.LocalAddr().(*net.UDPAddr).Port})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	r.Start()
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	payload := []byte("UDP 代理")
	_, _ = c.Write(payload)
	buffer := make([]byte, 65535)
	n, err := c.Read(buffer)
	if err != nil || !bytes.Equal(buffer[:n], payload) {
		t.Fatalf("UDP 真实转发失败: %q %v", buffer[:n], err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if <-source == c.LocalAddr().(*net.UDPAddr).Port {
		t.Fatal("目标看到的来源端口未经过独立代理 socket")
	}
}

func availableForwardPort(t *testing.T, protocol string) int {
	t.Helper()
	if protocol == "tcp" {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		return port
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return port
}
