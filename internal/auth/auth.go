package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

type Session struct {
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	Portal   string    `json:"-"`
	Username string    `json:"username"`
	CSRF     string    `json:"csrf_token"`
	Expires  time.Time `json:"-"`
}
type attempts struct {
	count int
	until time.Time
}
type UserRepository interface {
	UserByUsername(context.Context, string) (model.User, error)
}

type Manager struct {
	users                  UserRepository
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

// SetUsers 在启动时注入只读账号存储；两端仍共用同一限流器。
func (m *Manager) SetUsers(users UserRepository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users = users
}

func (m *Manager) Login(username, password string) (string, Session, bool) {
	return m.LoginPortal(context.Background(), model.RoleAdmin, username, password)
}

func (m *Manager) LoginPortal(ctx context.Context, portal, username, password string) (string, Session, bool) {
	userID, role, displayName := model.AdminID, model.RoleAdmin, m.username
	switch portal {
	case model.RoleAdmin:
		u, p := sha256.Sum256([]byte(username)), sha256.Sum256([]byte(password))
		validUser := subtle.ConstantTimeCompare(u[:], m.userHash[:])
		validPassword := subtle.ConstantTimeCompare(p[:], m.passwordHash[:])
		if validUser&validPassword != 1 {
			return "", Session{}, false
		}
	case model.RoleUser:
		m.mu.Lock()
		users := m.users
		m.mu.Unlock()
		if users == nil {
			return "", Session{}, false
		}
		user, err := users.UserByUsername(ctx, username)
		if err != nil || user.Role != model.RoleUser {
			// 无效账号同样执行标准派生，避免通过耗时枚举存在的普通用户。
			VerifyPassword(password, "pbkdf2-sha256$600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
			return "", Session{}, false
		}
		if !VerifyPassword(password, user.PasswordHash) {
			return "", Session{}, false
		}
		userID, role, displayName = user.ID, user.Role, user.Username
	default:
		return "", Session{}, false
	}
	token, csrf := random(), random()
	s := Session{UserID: userID, Role: role, Portal: portal, Username: displayName, CSRF: csrf, Expires: time.Now().Add(12 * time.Hour)}
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

// GetPortal 同时核对会话所属端与角色，复制另一端 Cookie 也不能越权。
func (m *Manager) GetPortal(token, portal string) (Session, bool) {
	s, ok := m.Get(token)
	return s, ok && (portal == model.RoleAdmin || portal == model.RoleUser) && s.Portal == portal && s.Role == portal
}

func (m *Manager) LogoutPortal(token, portal string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := sha256.Sum256([]byte(token))
	if s, ok := m.sessions[key]; ok && s.Portal == portal && s.Role == portal {
		delete(m.sessions, key)
	}
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
