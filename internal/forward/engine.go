// Package forward 提供受限额约束、可等待关闭的 TCP 与 UDP 普通代理。
package forward

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

type Limits struct {
	Global      int
	PerRule     int
	UDPIdle     time.Duration
	DialTimeout time.Duration
}

type Engine struct {
	limits  Limits
	active  atomic.Int64
	mu      sync.Mutex
	budgets map[string]*ruleBudget
}

// ruleBudget 由同一逻辑规则的全部预备与运行实例共享，字段统一受 Engine.mu 保护。
type ruleBudget struct {
	id     string
	refs   int
	active int
}

func New(limits Limits) *Engine {
	if limits.Global <= 0 {
		limits.Global = 4096
	}
	if limits.PerRule <= 0 {
		limits.PerRule = 1024
	}
	if limits.UDPIdle <= 0 {
		limits.UDPIdle = time.Minute
	}
	if limits.DialTimeout <= 0 {
		limits.DialTimeout = 5 * time.Second
	}
	return &Engine{limits: limits, budgets: make(map[string]*ruleBudget)}
}

func (e *Engine) retainBudget(id string) *ruleBudget {
	e.mu.Lock()
	defer e.mu.Unlock()
	budget := e.budgets[id]
	if id == "" || budget == nil {
		budget = &ruleBudget{id: id}
		if id != "" {
			e.budgets[id] = budget
		}
	}
	budget.refs++
	return budget
}

func (e *Engine) releaseBudget(budget *ruleBudget) {
	e.mu.Lock()
	defer e.mu.Unlock()
	budget.refs--
	if budget.refs == 0 && budget.id != "" {
		delete(e.budgets, budget.id)
	}
}

func (e *Engine) reserveSession(budget *ruleBudget) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if budget.active >= e.limits.PerRule || e.active.Load() >= int64(e.limits.Global) {
		return false
	}
	budget.active++
	e.active.Add(1)
	return true
}

func (e *Engine) releaseSession(budget *ruleBudget) {
	e.mu.Lock()
	defer e.mu.Unlock()
	budget.active--
	e.active.Add(-1)
}

type generation struct {
	rule     model.Rule
	ctx      context.Context
	cancel   context.CancelFunc
	sessions map[*session]struct{}
	udp      map[string]*session
	wg       sync.WaitGroup
}

func newGeneration(rule model.Rule) *generation {
	ctx, cancel := context.WithCancel(context.Background())
	return &generation{rule: rule, ctx: ctx, cancel: cancel, sessions: make(map[*session]struct{}), udp: make(map[string]*session)}
}

type Runtime struct {
	engine       *Engine
	budget       *ruleBudget
	op           sync.Mutex
	mu           sync.Mutex
	current      *generation
	tcp          *net.TCPListener
	udp          *net.UDPConn
	started      bool
	stopped      bool
	err          error
	failedSignal chan struct{}
	loops        sync.WaitGroup
	lastLog      time.Time
	active       int
	udpWrite     sync.Mutex
}

// Prepare 只占用监听端口；提交持久化配置后调用 Start 才开始处理数据。
func (e *Engine) Prepare(rule model.Rule) (*Runtime, error) {
	ip := net.ParseIP(rule.ListenHost)
	if ip == nil {
		return nil, errors.New("监听地址必须是 IP 地址")
	}
	family := "6"
	if ip.To4() != nil {
		family = "4"
	}
	address := net.JoinHostPort(rule.ListenHost, strconv.Itoa(rule.ListenPort))
	r := &Runtime{engine: e, current: newGeneration(rule), failedSignal: make(chan struct{})}
	var err error
	switch rule.Protocol {
	case "tcp":
		var addr *net.TCPAddr
		addr, err = net.ResolveTCPAddr("tcp"+family, address)
		if err == nil {
			r.tcp, err = net.ListenTCP("tcp"+family, addr)
		}
	case "udp":
		var addr *net.UDPAddr
		addr, err = net.ResolveUDPAddr("udp"+family, address)
		if err == nil {
			r.udp, err = net.ListenUDP("udp"+family, addr)
		}
	default:
		err = errors.New("协议必须是 TCP 或 UDP")
	}
	if err != nil {
		r.current.cancel()
		return nil, fmt.Errorf("无法监听 %s: %w", address, err)
	}
	r.budget = e.retainBudget(rule.ID)
	return r, nil
}

func (r *Runtime) Start() {
	r.op.Lock()
	defer r.op.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.stopped {
		return
	}
	r.started = true
	r.loops.Add(1)
	if r.tcp != nil {
		go r.acceptTCP()
	} else {
		go r.acceptUDP()
	}
}

// Update 保留监听器并换代；返回前旧拨号、连接和 UDP 会话均已退出。
// 调用方只在监听地址和协议不变且转发参数改变时使用此方法。
func (r *Runtime) Update(rule model.Rule) {
	r.op.Lock()
	defer r.op.Unlock()
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	old := r.current
	old.cancel()
	r.current = newGeneration(rule)
	r.mu.Unlock()
	r.drain(old)
}

func (r *Runtime) Stop() {
	r.op.Lock()
	defer r.op.Unlock()
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	g := r.current
	g.cancel()
	r.mu.Unlock()
	if r.tcp != nil {
		_ = r.tcp.Close()
	}
	if r.udp != nil {
		_ = r.udp.Close()
	}
	r.drain(g)
	r.loops.Wait()
	r.engine.releaseBudget(r.budget)
}

func (r *Runtime) Error() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Failed 只在监听发生不可恢复故障时关闭；正常停止不触发故障通知。
// 调用方收到通知后可安全调用 Error，并由独立协程协调组内 Stop。
func (r *Runtime) Failed() <-chan struct{} { return r.failedSignal }

func (r *Runtime) drain(g *generation) {
	r.mu.Lock()
	all := make([]*session, 0, len(g.sessions))
	for s := range g.sessions {
		all = append(all, s)
	}
	r.mu.Unlock()
	for _, s := range all {
		s.close()
	}
	g.wg.Wait()
}

func (r *Runtime) failed(err error) {
	r.mu.Lock()
	if r.stopped || r.err != nil {
		r.mu.Unlock()
		return
	}
	r.err = fmt.Errorf("监听器故障: %w", err)
	close(r.failedSignal)
	g := r.current
	g.cancel()
	r.mu.Unlock()
	if r.tcp != nil {
		_ = r.tcp.Close()
	}
	if r.udp != nil {
		_ = r.udp.Close()
	}
	// 不在监听循环里等待自身退出；会话完成后由 Stop 等待循环。
	r.drain(g)
}

// reserve 在运行态锁内分配额度与登记会话，避免关闭与 WaitGroup.Add 交错。
func (r *Runtime) reserve(g *generation, key string, client net.Conn, peer *net.UDPAddr) *session {
	if !r.engine.reserveSession(r.budget) {
		return nil
	}
	s := &session{runtime: r, generation: g, client: client, peer: peer, key: key, packets: make(chan []byte, 64), done: make(chan struct{})}
	g.sessions[s] = struct{}{}
	r.active++
	if key != "" {
		g.udp[key] = s
	}
	g.wg.Add(1)
	return s
}

func (r *Runtime) warn(code string, err error) {
	r.mu.Lock()
	if time.Since(r.lastLog) < time.Second {
		r.mu.Unlock()
		return
	}
	r.lastLog = time.Now()
	id := r.current.rule.ID
	r.mu.Unlock()
	slog.Warn("转发会话失败", "rule_id", id, "error_code", code, "error", err)
}

type session struct {
	runtime    *Runtime
	generation *generation
	client     net.Conn
	peer       *net.UDPAddr
	key        string
	packets    chan []byte
	done       chan struct{}
	mu         sync.Mutex
	closed     bool
	upstream   net.Conn
}

func (s *session) attach(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.generation.ctx.Err() != nil {
		_ = conn.Close()
		return false
	}
	s.upstream = conn
	return true
}

func (s *session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.done)
	if s.client != nil {
		_ = s.client.Close()
	}
	if s.upstream != nil {
		_ = s.upstream.Close()
	}
}

func (s *session) finish() {
	s.close()
	r, g := s.runtime, s.generation
	r.mu.Lock()
	delete(g.sessions, s)
	r.active--
	if s.key != "" && g.udp[s.key] == s {
		delete(g.udp, s.key)
	}
	r.mu.Unlock()
	r.engine.releaseSession(r.budget)
	g.wg.Done()
}

func (r *Runtime) dial(g *generation) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(g.ctx, r.engine.limits.DialTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, g.rule.TargetHost)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("目标域名没有可用地址")
	}
	for _, ip := range ips {
		if ip.IP.IsUnspecified() || ip.IP.IsMulticast() {
			return nil, errors.New("目标解析到未指定地址或组播地址")
		}
		if directLoop(g.rule, ip.IP) {
			return nil, errors.New("目标解析到自身监听地址，已阻止直接回环")
		}
	}
	var last error
	for _, ip := range ips {
		host := ip.IP.String()
		if ip.Zone != "" {
			host += "%" + ip.Zone
		}
		dialer := net.Dialer{KeepAlive: 30 * time.Second}
		conn, err := dialer.DialContext(ctx, g.rule.Protocol, net.JoinHostPort(host, strconv.Itoa(g.rule.TargetPort)))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func directLoop(rule model.Rule, target net.IP) bool {
	if rule.ListenPort != rule.TargetPort {
		return false
	}
	listen := net.ParseIP(rule.ListenHost)
	if listen == nil {
		return false
	}
	if !listen.IsUnspecified() {
		return listen.Equal(target)
	}
	if (listen.To4() != nil) != (target.To4() != nil) {
		return false
	}
	if target.IsLoopback() || target.IsUnspecified() {
		return true
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return true
	}
	for _, address := range addresses {
		if ip, _, err := net.ParseCIDR(address.String()); err == nil && ip.Equal(target) {
			return true
		}
	}
	return false
}
