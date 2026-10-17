package rules

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

type signalledRuntime struct {
	mu       sync.Mutex
	err      error
	failed   chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
}

func newSignalledRuntime() *signalledRuntime {
	return &signalledRuntime{failed: make(chan struct{}), stopped: make(chan struct{})}
}
func (r *signalledRuntime) Start()                  {}
func (r *signalledRuntime) Update(model.Rule)       {}
func (r *signalledRuntime) Stop()                   { r.stopOnce.Do(func() { close(r.stopped) }) }
func (r *signalledRuntime) Error() error            { r.mu.Lock(); defer r.mu.Unlock(); return r.err }
func (r *signalledRuntime) Failed() <-chan struct{} { return r.failed }
func (r *signalledRuntime) fail() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == nil {
		r.err = errors.New("注入真实监听故障信号")
		close(r.failed)
	}
}
func waitStopped(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("故障协调未及时停止组内监听")
	}
}

func groupService(t *testing.T) (*Service, *signalledRuntime, *signalledRuntime) {
	t.Helper()
	tcp, udp := newSignalledRuntime(), newSignalledRuntime()
	r := model.Rule{ID: "combined", Name: "组合", Protocol: model.ProtocolBoth, Enabled: true}
	s := &Service{items: map[string]model.Rule{r.ID: r}, running: make(map[string]*runtimeGroup)}
	g := &runtimeGroup{members: map[string]runtimeHandle{"tcp": tcp, "udp": udp}, done: make(chan struct{})}
	s.mu.Lock()
	s.startGroup(r.ID, g)
	s.mu.Unlock()
	t.Cleanup(func() { _ = s.Close() })
	return s, tcp, udp
}

func TestListenerFailureStopsPeerWithoutListRequest(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, tcp, udp := groupService(t)
			if transport == "tcp" {
				tcp.fail()
			} else {
				udp.fail()
			}
			// 在读取列表前等待两个停止信号，防止只在 view 时被动释放资源。
			waitStopped(t, tcp.stopped)
			waitStopped(t, udp.stopped)
			rows := s.List()
			if len(rows) != 1 || rows[0].Status != "error" || !strings.Contains(rows[0].LastError, strings.ToUpper(transport)) {
				t.Fatalf("组故障状态不正确：%+v", rows)
			}
			s.mu.Lock()
			remaining := len(s.running)
			s.mu.Unlock()
			if remaining != 0 {
				t.Fatal("故障组仍留在运行集合")
			}
		})
	}
}

func TestConcurrentGroupFailureAndServiceClose(t *testing.T) {
	for range 32 {
		s, tcp, udp := groupService(t)
		var wg sync.WaitGroup
		for _, action := range []func(){tcp.fail, udp.fail, func() { _ = s.Close() }} {
			wg.Add(1)
			go func() { defer wg.Done(); action() }()
		}
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("故障与关闭并发发生时死锁")
		}
		waitStopped(t, tcp.stopped)
		waitStopped(t, udp.stopped)
	}
}
