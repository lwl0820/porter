package auth

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCredentialsSessionsAndDigestStorage(t *testing.T) {
	m := New("管理员", "测试密码")
	for _, credentials := range [][2]string{{"管理员", "错误"}, {"错误", "测试密码"}, {"", ""}} {
		if token, _, ok := m.Login(credentials[0], credentials[1]); ok || token != "" {
			t.Fatal("错误凭据签发了会话")
		}
	}
	token, session, ok := m.Login("管理员", "测试密码")
	if !ok || session.Username != "管理员" || len(token) != 64 || len(session.CSRF) != 64 || token == session.CSRF {
		t.Fatal("有效凭据没有生成独立高熵 token")
	}
	if _, ok := m.sessions[sha256.Sum256([]byte(token))]; !ok {
		t.Fatal("服务端未以 token 摘要索引会话")
	}
	if remaining := time.Until(session.Expires); remaining < 11*time.Hour || remaining > 12*time.Hour {
		t.Fatal("会话有效期不正确")
	}
	if got, ok := m.Get(token); !ok || got != session {
		t.Fatal("有效 token 无法获取原会话")
	}
	for _, invalid := range []string{"", token[:63], strings.Repeat("x", 64), token + "x"} {
		if _, ok := m.Get(invalid); ok {
			t.Fatal("非法 token 命中了会话")
		}
	}
	m.Logout(token)
	m.Logout(token)
	if _, ok := m.Get(token); ok {
		t.Fatal("注销后 token 仍有效")
	}
	restarted := New("管理员", "测试密码")
	if _, ok := restarted.Get(token); ok {
		t.Fatal("新管理器接受了旧进程的会话")
	}
}

func TestSessionExpirationAndBoundedRetention(t *testing.T) {
	m := New("admin", "password")
	token, session, _ := m.Login("admin", "password")
	session.Expires = time.Now().Add(-time.Nanosecond)
	m.sessions[sha256.Sum256([]byte(token))] = session
	if _, ok := m.Get(token); ok || len(m.sessions) != 0 {
		t.Fatal("过期会话未拒绝或未回收")
	}
	first, _, _ := m.Login("admin", "password")
	for range 128 {
		if _, _, ok := m.Login("admin", "password"); !ok {
			t.Fatal("有效登录失败")
		}
	}
	if len(m.sessions) != 128 {
		t.Fatalf("会话数量未限制为 128：%d", len(m.sessions))
	}
	if _, ok := m.Get(first); ok {
		t.Fatal("达到上限后最旧会话未淘汰")
	}
}

func TestLoginLimitsPerSourceGlobalAndExpiry(t *testing.T) {
	m := New("admin", "password")
	for range 10 {
		if !m.AllowLogin("192.0.2.1") {
			t.Fatal("来源正常额度提前耗尽")
		}
	}
	if m.AllowLogin("192.0.2.1") {
		t.Fatal("来源额度未生效")
	}
	if !m.AllowLogin("192.0.2.2") {
		t.Fatal("单一来源耗尽影响了其他来源")
	}
	m.byIP["192.0.2.1"] = attempts{count: 10, until: time.Now().Add(-time.Second)}
	if !m.AllowLogin("192.0.2.1") {
		t.Fatal("来源限流过期后没有恢复")
	}
	global := New("admin", "password")
	for i := range 60 {
		if !global.AllowLogin(fmt.Sprintf("192.0.2.%d", i)) {
			t.Fatal("全局额度提前耗尽")
		}
	}
	if global.AllowLogin("198.51.100.1") {
		t.Fatal("不同来源绕过全局限额")
	}
	global.global.until = time.Now().Add(-time.Second)
	if !global.AllowLogin("198.51.100.1") {
		t.Fatal("全局限流窗口过期后没有恢复")
	}
}

func TestSourceTrackingBoundAndExpiredEntryCleanup(t *testing.T) {
	m := New("admin", "password")
	for i := range 4096 {
		m.byIP[fmt.Sprint(i)] = attempts{until: time.Now().Add(time.Minute)}
	}
	if m.AllowLogin("新来源") {
		t.Fatal("来源记录超过上限仍继续分配")
	}
	if !m.AllowLogin("0") {
		t.Fatal("来源记录满后拒绝已有来源")
	}
	m.byIP["0"] = attempts{until: time.Now().Add(-time.Second)}
	if !m.AllowLogin("新来源") {
		t.Fatal("过期来源未回收")
	}
	if len(m.byIP) != 4096 {
		t.Fatalf("来源记录数量异常：%d", len(m.byIP))
	}
}

func TestConcurrentSessionAccessAndRevocation(t *testing.T) {
	m := New("admin", "password")
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				token, _, ok := m.Login("admin", "password")
				if !ok {
					t.Error("并发登录失败")
					return
				}
				if _, ok = m.Get(token); !ok {
					t.Error("并发登录后会话丢失")
					return
				}
				m.AllowLogin(fmt.Sprintf("192.0.2.%d", i))
				m.Logout(token)
				if _, ok = m.Get(token); ok {
					t.Error("并发注销后会话仍有效")
					return
				}
			}
		}()
	}
	wg.Wait()
	if len(m.sessions) != 0 {
		t.Fatal("并发访问后会话没有全部撤销")
	}
}
