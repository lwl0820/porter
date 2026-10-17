package forward

import (
	"errors"
	"time"
)

func (r *Runtime) acceptUDP() {
	defer r.loops.Done()
	// 大于任何合法 IPv4/IPv6 UDP 载荷，避免截断后转发不完整数据报。
	buffer := make([]byte, 65535)
	for {
		n, peer, err := r.udp.ReadFromUDP(buffer)
		if err != nil {
			r.failed(err)
			return
		}
		key := peer.String()
		r.mu.Lock()
		g := r.current
		if r.stopped || g.ctx.Err() != nil {
			r.mu.Unlock()
			continue
		}
		s := g.udp[key]
		fresh := false
		if s == nil {
			s = r.reserve(g, key, nil, peer)
			fresh = s != nil
		}
		r.mu.Unlock()
		if s == nil {
			r.warn("session_limit", errors.New("会话数量达到上限"))
			continue
		}
		packet := append([]byte{}, buffer[:n]...)
		select {
		case s.packets <- packet:
		case <-s.done:
		default:
			r.warn("udp_queue_full", errors.New("UDP 会话待发送队列已满"))
		}
		if fresh {
			go s.runUDP()
		}
	}
}

func (s *session) runUDP() {
	defer s.finish()
	upstream, err := s.runtime.dial(s.generation)
	if err != nil {
		s.runtime.warn("target_dial", err)
		return
	}
	if !s.attach(upstream) {
		return
	}
	idle := s.runtime.engine.limits.UDPIdle
	_ = upstream.SetReadDeadline(time.Now().Add(idle))
	repliesDone := make(chan struct{})
	go func() {
		defer close(repliesDone)
		defer s.close()
		buffer := make([]byte, 65535)
		for {
			n, err := upstream.Read(buffer)
			if err != nil {
				return
			}
			_ = upstream.SetReadDeadline(time.Now().Add(idle))
			// 共享监听器串行设置写截止时间，单个不可写目标不能阻塞更新等待。
			s.runtime.udpWrite.Lock()
			if s.generation.ctx.Err() != nil {
				s.runtime.udpWrite.Unlock()
				return
			}
			_ = s.runtime.udp.SetWriteDeadline(time.Now().Add(time.Second))
			written, writeErr := s.runtime.udp.WriteToUDP(buffer[:n], s.peer)
			s.runtime.traffic.Add(false, written)
			err = writeErr
			s.runtime.udpWrite.Unlock()
			if err != nil {
				return
			}
		}
	}()
	defer func() { s.close(); <-repliesDone }()
	for {
		select {
		case <-s.done:
			return
		case <-s.generation.ctx.Done():
			return
		case packet := <-s.packets:
			_ = upstream.SetReadDeadline(time.Now().Add(idle))
			n, err := upstream.Write(packet)
			s.runtime.traffic.Add(true, n)
			if err != nil {
				return
			}
		}
	}
}
