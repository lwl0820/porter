package rules

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
)

// runtimeGroup 的成员集合创建后不变，所有切换和关闭均由 Service 锁串行保护。
type runtimeGroup struct {
	members map[string]runtimeHandle
	done    chan struct{}
	retired bool
}

// runtimeHandle 将组生命周期与具体 socket 解耦，便于验证跨监听故障协调。
type runtimeHandle interface {
	Start()
	Update(model.Rule)
	Stop()
	Error() error
	Failed() <-chan struct{}
}

type listenerError struct {
	protocol string
	err      error
}

func (e *listenerError) Error() string {
	return fmt.Sprintf("%s 监听失败：%v", strings.ToUpper(e.protocol), e.err)
}
func (e *listenerError) Unwrap() error { return e.err }

func physicalRule(r model.Rule, transport string) model.Rule { r.Protocol = transport; return r }
func (g *runtimeGroup) retire() {
	if !g.retired {
		g.retired = true
		close(g.done)
	}
}
func (g *runtimeGroup) stop() {
	g.retire()
	for _, rt := range g.members {
		rt.Stop()
	}
}
func (g *runtimeGroup) failure() error {
	for _, transport := range []string{model.ProtocolTCP, model.ProtocolUDP} {
		if rt := g.members[transport]; rt != nil {
			if err := rt.Error(); err != nil {
				return &listenerError{protocol: transport, err: err}
			}
		}
	}
	return nil
}

// prepareGroup 只准备新增监听，失败时仅释放新增资源，不触碰公共旧监听。
func (s *Service) prepareGroup(r model.Rule, reusable *runtimeGroup) (*runtimeGroup, error) {
	g := &runtimeGroup{members: make(map[string]runtimeHandle), done: make(chan struct{})}
	var added []runtimeHandle
	for _, transport := range model.Transports(r.Protocol) {
		if reusable != nil && reusable.members[transport] != nil {
			g.members[transport] = reusable.members[transport]
			continue
		}
		var meter forward.Meter
		if m := s.meters[r.OwnerID]; m != nil {
			meter = m
		}
		rt, err := s.engine.PrepareMeter(physicalRule(r, transport), s.traffic[r.ID], meter)
		if err != nil {
			for _, pending := range added {
				pending.Stop()
			}
			return nil, &listenerError{protocol: transport, err: err}
		}
		g.members[transport] = rt
		added = append(added, rt)
	}
	return g, nil
}

func releaseAdded(next, old *runtimeGroup) {
	if next == nil {
		return
	}
	next.retire()
	for transport, rt := range next.members {
		if old == nil || old.members[transport] != rt {
			rt.Stop()
		}
	}
}

func (s *Service) startGroup(id string, g *runtimeGroup) {
	s.running[id] = g
	for _, rt := range g.members {
		rt.Start()
	}
	var tcpFailed, udpFailed <-chan struct{}
	if rt := g.members[model.ProtocolTCP]; rt != nil {
		tcpFailed = rt.Failed()
	}
	if rt := g.members[model.ProtocolUDP]; rt != nil {
		udpFailed = rt.Failed()
	}
	s.monitors.Add(1)
	go func() {
		defer s.monitors.Done()
		select {
		case <-g.done:
			return
		case <-tcpFailed:
		case <-udpFailed:
		}
		// 监听循环只发信号，协调组在此独立协程中等待所有 Runtime 关闭。
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || s.running[id] != g {
			return
		}
		if err := g.failure(); err != nil {
			s.failGroup(s.items[id], g, err)
		}
	}()
}

func (s *Service) failGroup(r model.Rule, g *runtimeGroup, err error) {
	g.stop()
	delete(s.running, r.ID)
	s.setError(r, err)
}

func listenerMessage(err error) string {
	var listener *listenerError
	if errors.As(err, &listener) {
		return fmt.Sprintf("%s 监听失败，请检查地址、端口占用和权限", strings.ToUpper(listener.protocol))
	}
	var validation *Error
	if errors.As(err, &validation) {
		return validation.Message
	}
	return "监听失败，请检查地址、端口占用和权限"
}
