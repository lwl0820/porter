package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/lwl0820/porter/internal/model"
)

type userFixture struct{ user model.User }

func (f userFixture) UserByUsername(_ context.Context, name string) (model.User, error) {
	if name != f.user.Username {
		return model.User{}, errors.New("账号不存在")
	}
	return f.user, nil
}

func TestPasswordsSaltedEncodingAndInvalidValues(t *testing.T) {
	first, err := HashPassword("用户密码")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("用户密码")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || strings.Contains(first, "用户密码") {
		t.Fatal("随机盐未隔离密码派生结果")
	}
	if !VerifyPassword("用户密码", first) || VerifyPassword("错误密码", first) {
		t.Fatal("正确与错误密码校验错误")
	}
	for _, value := range []string{"", "明文", "pbkdf2-sha256$0$x$x", "pbkdf2-sha256$999999999$x$x", strings.Replace(first, "pbkdf2-sha256", "unknown", 1), first + "$extra"} {
		if VerifyPassword("用户密码", value) {
			t.Fatal("非法密码编码被接受")
		}
	}
	for _, value := range []string{"", " ", strings.Repeat("x", 1025)} {
		if _, err := HashPassword(value); err == nil {
			t.Fatal("无效密码未拒绝")
		}
	}
}

func TestPortalsIsolateIdentityTokensAndLogout(t *testing.T) {
	encoded, err := HashPassword("用户密码")
	if err != nil {
		t.Fatal(err)
	}
	u := model.User{ID: "user-1", Username: "用户", Role: model.RoleUser, PasswordHash: encoded}
	m := New("管理员", "管理员密码")
	m.SetUsers(userFixture{user: u})
	ctx := context.Background()
	adminToken, admin, ok := m.LoginPortal(ctx, model.RoleAdmin, "管理员", "管理员密码")
	if !ok || admin.UserID != model.AdminID || admin.Role != model.RoleAdmin {
		t.Fatal("管理员身份错误")
	}
	userToken, user, ok := m.LoginPortal(ctx, model.RoleUser, "用户", "用户密码")
	if !ok || user.UserID != u.ID || user.Role != model.RoleUser || admin.CSRF == user.CSRF {
		t.Fatal("用户身份或CSRF隔离错误")
	}
	if _, ok = m.GetPortal(adminToken, model.RoleUser); ok {
		t.Fatal("管理token被用户端接受")
	}
	if _, ok = m.GetPortal(userToken, model.RoleAdmin); ok {
		t.Fatal("用户token被管理端接受")
	}
	if _, _, ok = m.LoginPortal(ctx, model.RoleAdmin, "用户", "用户密码"); ok {
		t.Fatal("用户能登录管理端")
	}
	if _, _, ok = m.LoginPortal(ctx, model.RoleUser, "管理员", "管理员密码"); ok {
		t.Fatal("管理员能登录用户端")
	}
	if _, _, ok = m.LoginPortal(ctx, "unknown", "管理员", "管理员密码"); ok {
		t.Fatal("未知端签发会话")
	}
	m.LogoutPortal(userToken, model.RoleAdmin)
	if _, ok = m.GetPortal(userToken, model.RoleUser); !ok {
		t.Fatal("错误端能注销其他端会话")
	}
	m.LogoutPortal(adminToken, model.RoleAdmin)
	if _, ok = m.GetPortal(adminToken, model.RoleAdmin); ok {
		t.Fatal("管理端未注销")
	}
	if _, ok = m.GetPortal(userToken, model.RoleUser); !ok {
		t.Fatal("管理注销影响用户端")
	}
	data, err := json.Marshal(u)
	if err != nil || strings.Contains(string(data), encoded) || strings.Contains(string(data), "password") {
		t.Fatal("用户响应泄露派生密码")
	}
	data, err = json.Marshal(user)
	if err != nil || strings.Contains(string(data), "Portal") || !strings.Contains(string(data), "user_id") {
		t.Fatal("会话字段不符合契约")
	}
}
