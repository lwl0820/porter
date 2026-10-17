package forward

import (
	"errors"
	"io"
	"net"
)

func (r *Runtime) acceptTCP() {
	defer r.loops.Done()
	for {
		client, err := r.tcp.AcceptTCP()
		if err != nil {
			r.failed(err)
			return
		}
		_ = client.SetKeepAlive(true)
		r.mu.Lock()
		g := r.current
		var s *session
		if !r.stopped && g.ctx.Err() == nil {
			s = r.reserve(g, "", client, nil)
		}
		r.mu.Unlock()
		if s == nil {
			_ = client.Close()
			r.warn("session_limit", errors.New("会话数量达到上限"))
			continue
		}
		go s.runTCP()
	}
}

func (s *session) runTCP() {
	defer s.finish()
	upstream, err := s.runtime.dial(s.generation)
	if err != nil {
		s.runtime.warn("target_dial", err)
		return
	}
	if !s.attach(upstream) {
		return
	}
	finished := make(chan struct{}, 2)
	copyDirection := func(dst, src net.Conn, upload bool) {
		_, err := io.Copy(trafficWriter{writer: dst, traffic: s.runtime.traffic, upload: upload, meter: s.runtime.meter, ctx: s.generation.ctx}, struct{ io.Reader }{src})
		if err != nil {
			s.close()
		} else if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		finished <- struct{}{}
	}
	go copyDirection(upstream, s.client, true)
	go copyDirection(s.client, upstream, false)
	<-finished
	<-finished
}
