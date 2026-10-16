package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

type Session struct {
	Username string    `json:"username"`
	CSRF     string    `json:"csrf_token"`
	Expires  time.Time `json:"-"`
}
type attempts struct {
	count int
	until time.Time
}
type Manager struct {
	mu                     sync.Mutex
	username               string
	userHash, passwordHash [32]byte
	sessions               map[[32]byte]Session
	byIP                   map[string]attempts
	global                 attempts
}

func New(username, password string) *Manager {
	return &Manager{username: username, userHash: sha256.Sum256([]byte(username)), passwordHash: sha256.Sum256([]byte(password)), sessions: make(map[[32]byte]Session), byIP: make(map[string]attempts)}
}

func (m *Manager) AllowLogin(ip string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.byIP {
		if now.After(v.until) {
			delete(m.byIP, k)
		}
	}
	if now.After(m.global.until) {
		m.global = attempts{until: now.Add(time.Minute)}
	}
	a, exists := m.byIP[ip]
	if !exists {
		if len(m.byIP) >= 4096 {
			return false
		}
		a.until = now.Add(time.Minute)
	}
	if m.global.count >= 60 || a.count >= 10 {
		return false
	}
	m.global.count++
	a.count++
	m.byIP[ip] = a
	return true
}

func (m *Manager) Login(username, password string) (string, Session, bool) {
	u, p := sha256.Sum256([]byte(username)), sha256.Sum256([]byte(password))
	validUser := subtle.ConstantTimeCompare(u[:], m.userHash[:])
	validPassword := subtle.ConstantTimeCompare(p[:], m.passwordHash[:])
	if validUser&validPassword != 1 {
		return "", Session{}, false
	}
	token, csrf := random(), random()
	s := Session{Username: m.username, CSRF: csrf, Expires: time.Now().Add(12 * time.Hour)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune()
	if len(m.sessions) >= 128 {
		var oldest [32]byte
		var expires time.Time
		for k, v := range m.sessions {
			if expires.IsZero() || v.Expires.Before(expires) {
				oldest = k
				expires = v.Expires
			}
		}
		delete(m.sessions, oldest)
	}
	m.sessions[sha256.Sum256([]byte(token))] = s
	return token, s, true
}

func (m *Manager) Get(token string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune()
	if len(token) != 64 {
		return Session{}, false
	}
	s, ok := m.sessions[sha256.Sum256([]byte(token))]
	return s, ok
}
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sha256.Sum256([]byte(token)))
}
func (m *Manager) prune() {
	now := time.Now()
	for k, v := range m.sessions {
		if !now.Before(v.Expires) {
			delete(m.sessions, k)
		}
	}
}
func random() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("系统随机数源不可用")
	}
	return hex.EncodeToString(b[:])
}
