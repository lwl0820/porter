package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/auth"
	"github.com/lwl0820/porter/internal/model"
)

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func decodeValue[T any](t *testing.T, r apiResponse) T {
	t.Helper()
	var v T
	if e := json.Unmarshal(r.body, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func userPortalFixture(t *testing.T, admin *apiFixture, username string) *apiFixture {
	t.Helper()
	f := &apiFixture{server: admin.server, db: admin.db, service: admin.service, manager: admin.manager}
	response := f.request(t, "POST", "/api/user/auth/login", jsonBody(t, map[string]string{"username": username, "password": "测试密码123"}), nil, false)
	expectStatus(t, response, 200)
	session := decodeValue[auth.Session](t, response)
	if session.Role != model.RoleUser || session.UserID == "" {
		t.Fatal("用户登录身份错误")
	}
	f.csrf = session.CSRF
	for _, c := range response.cookies {
		if c.Name == "porter_user_session" {
			f.cookie = c
		}
	}
	if f.cookie == nil || f.cookie.Path != "/api/user" {
		t.Fatal("用户 Cookie 未隔离")
	}
	return f
}
func provisionAPIUsers(t *testing.T) (*apiFixture, *apiFixture, *apiFixture, model.Account, model.Account) {
	t.Helper()
	f := newAPI(t, "", false)
	f.login(t)
	settings := f.request(t, "PUT", "/api/admin/settings", `{"port_start":20000,"port_end":20099}`, map[string]string{"If-Match": `"1"`}, true)
	expectStatus(t, settings, 200)
	pResp := f.request(t, "POST", "/api/admin/plans", `{"name":"测试套餐","port_count":10,"traffic_limit_bytes":"9007199254740993","accounting_mode":"max","reset_mode":"fixed_day","reset_day":31}`, nil, true)
	expectStatus(t, pResp, 201)
	p := decodeValue[model.Plan](t, pResp)
	accounts := make([]model.Account, 0, 2)
	for i := 1; i <= 2; i++ {
		r := f.request(t, "POST", "/api/admin/users", jsonBody(t, model.UserInput{Username: "用户" + strconv.Itoa(i), Password: "测试密码123", PlanID: p.ID}), nil, true)
		expectStatus(t, r, 201)
		if bytes.Contains(r.body, []byte("password")) || bytes.Contains(r.body, []byte("pbkdf2")) {
			t.Fatal("用户响应泄露凭据")
		}
		accounts = append(accounts, decodeValue[model.Account](t, r))
	}
	return f, userPortalFixture(t, f, "用户1"), userPortalFixture(t, f, "用户2"), accounts[0], accounts[1]
}
func TestIndependentPortalsAndCrossUserRuleOperations(t *testing.T) {
	f, one, two, a, b := provisionAPIUsers(t)
	disabled := false
	in := model.Input{Name: "归属规则", Protocol: "tcp+udp", ListenHost: "127.0.0.1", ListenPort: a.User.PortStart, TargetHost: "127.0.0.1", TargetPort: 1, Enabled: &disabled}
	created := f.request(t, "POST", "/api/admin/rules", jsonBody(t, model.AdminRuleCreateInput{Input: in, OwnerID: a.User.ID}), nil, true)
	expectStatus(t, created, 201)
	r := decodeValue[model.Rule](t, created)
	if r.Owner == nil || r.Owner.Username != a.User.Username || r.OwnerID != a.User.ID {
		t.Fatalf("管理响应缺少权威归属：%s", created.body)
	}
	in.ListenPort = b.User.PortStart
	second := two.request(t, "POST", "/api/user/rules", inputJSON(t, in), nil, true)
	expectStatus(t, second, 201)
	r2 := decodeValue[model.Rule](t, second)
	if r2.Owner != nil || r2.OwnerID != b.User.ID {
		t.Fatal("用户规则归属错误")
	}
	in.ListenPort = 10001
	adminRule := f.request(t, "POST", "/api/admin/rules", inputJSON(t, in), nil, true)
	expectStatus(t, adminRule, 201)
	var list struct {
		Rules []model.Rule `json:"rules"`
	}
	for _, c := range []struct {
		client *apiFixture
		path   string
		count  int
	}{{f, "/api/admin/rules", 3}, {f, "/api/admin/rules?owner_id=" + a.User.ID, 1}, {f, "/api/admin/rules?owner_id=" + model.AdminID, 1}, {f, "/api/admin/rules?owner_id=missing", 0}, {one, "/api/user/rules", 1}, {one, "/api/user/rules?owner_id=" + b.User.ID, 1}} {
		resp := c.client.request(t, "GET", c.path, "", nil, true)
		expectStatus(t, resp, 200)
		list = decodeValue[struct {
			Rules []model.Rule `json:"rules"`
		}](t, resp)
		if len(list.Rules) != c.count {
			t.Fatalf("列表隔离或筛选错误：%s", resp.body)
		}
	}
	path := "/api/user/rules/" + r.ID
	headers := map[string]string{"If-Match": `"999"`}
	for _, action := range []struct{ method, path, body string }{{"PUT", path, inputJSON(t, in)}, {"DELETE", path, ""}, {"POST", path + "/retry", ""}, {"POST", path + "/traffic/reset", ""}} {
		expectCode(t, two.request(t, action.method, action.path, action.body, headers, true), 404, "not_found")
	}
	expectCode(t, one.request(t, "POST", "/api/user/rules", jsonBody(t, model.AdminRuleCreateInput{Input: in, OwnerID: b.User.ID}), nil, true), 400, "invalid_json")
	outside := in
	outside.ListenPort = a.User.PortEnd + 1
	expectCode(t, f.request(t, "POST", "/api/admin/rules", jsonBody(t, model.AdminRuleCreateInput{Input: outside, OwnerID: a.User.ID}), nil, true), 400, "validation_error")
	// 管理员编辑、启停、重试、展示清零和删除用户规则；用户持有旧版本不能覆盖。
	in.ListenPort = a.User.PortStart
	in.Name = "管理员编辑"
	updated := f.request(t, "PUT", "/api/admin/rules/"+r.ID, inputJSON(t, in), map[string]string{"If-Match": etag(r.Revision)}, true)
	expectStatus(t, updated, 200)
	r = decodeValue[model.Rule](t, updated)
	expectCode(t, one.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": `"1"`}, true), 412, "revision_mismatch")
	expectCode(t, f.request(t, "POST", "/api/admin/rules/"+r.ID+"/retry", "", map[string]string{"If-Match": etag(r.Revision)}, true), 409, "disabled")
	reset := f.request(t, "POST", "/api/admin/rules/"+r.ID+"/traffic/reset", "", map[string]string{"If-Match": etag(r.Revision)}, true)
	expectStatus(t, reset, 200)
	r = decodeValue[model.Rule](t, reset)
	if r.Owner == nil || r.OwnerID != a.User.ID {
		t.Fatal("管理员操作转移归属")
	}
	expectStatus(t, f.request(t, "DELETE", "/api/admin/rules/"+r.ID, "", map[string]string{"If-Match": etag(r.Revision)}, true), 204)
	expectCode(t, one.request(t, "DELETE", path, "", map[string]string{"If-Match": etag(r.Revision)}, true), 404, "not_found")
	account := one.request(t, "GET", "/api/user/account", "", nil, true)
	expectStatus(t, account, 200)
	got := decodeValue[model.Account](t, account)
	if got.User.ID != a.User.ID || got.Usage.LimitBytes != 9007199254740993 {
		t.Fatal("账户响应精度或归属错误")
	}
}
func TestPortalCookiesCSRFAndLogoutAreIndependent(t *testing.T) {
	admin, user, _, _, _ := provisionAPIUsers(t)
	for _, path := range []string{"/api/auth/session", "/api/rules", "/api/account"} {
		expectCode(t, admin.request(t, "GET", path, "", nil, true), 404, "not_found")
	}
	expectCode(t, user.request(t, "GET", "/api/admin/users", "", nil, true), 401, "unauthorized")
	expectCode(t, admin.request(t, "GET", "/api/user/account", "", nil, true), 401, "unauthorized")
	copied := &apiFixture{server: admin.server, cookie: &http.Cookie{Name: cookieName, Value: user.cookie.Value}, csrf: user.csrf}
	expectCode(t, copied.request(t, "GET", "/api/admin/users", "", nil, true), 403, "forbidden")
	copied.cookie = &http.Cookie{Name: "porter_user_session", Value: admin.cookie.Value}
	copied.csrf = admin.csrf
	expectCode(t, copied.request(t, "GET", "/api/user/account", "", nil, true), 403, "forbidden")
	// 浏览器同时携带两端Cookie，服务端只读取当前端。
	for _, c := range []struct {
		path     string
		selected *apiFixture
	}{{"/api/admin/auth/session", admin}, {"/api/user/auth/session", user}} {
		req, _ := http.NewRequest("GET", admin.server.URL+c.path, nil)
		req.AddCookie(user.cookie)
		req.AddCookie(admin.cookie)
		response, err := admin.server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		var s auth.Session
		json.Unmarshal(body, &s)
		if response.StatusCode != 200 || s.CSRF != c.selected.csrf {
			t.Fatalf("同存Cookie串用：%s", body)
		}
	}
	expectCode(t, user.request(t, "POST", "/api/user/auth/logout", "", map[string]string{"X-CSRF-Token": admin.csrf}, true), 403, "csrf_invalid")
	expectStatus(t, user.request(t, "POST", "/api/user/auth/logout", "", nil, true), 204)
	expectStatus(t, admin.request(t, "GET", "/api/admin/auth/session", "", nil, true), 200)
	expectCode(t, user.request(t, "GET", "/api/user/auth/session", "", nil, true), 401, "unauthorized")
	expectCode(t, admin.request(t, "POST", "/api/user/auth/login", `{"username":"管理员","password":"测试密码"}`, nil, false), 401, "credentials_invalid")
	expectCode(t, admin.request(t, "POST", "/api/admin/auth/login", `{"username":"用户1","password":"测试密码123"}`, nil, false), 401, "credentials_invalid")
}
func TestPlanSettingsValidationAndUserPortRange(t *testing.T) {
	f := newAPI(t, "", false)
	f.login(t)
	for _, body := range []string{`{"name":"无固定日","port_count":1,"traffic_limit_bytes":"1","accounting_mode":"both","reset_mode":"fixed_day"}`, `{"name":"多余日","port_count":1,"traffic_limit_bytes":"1","accounting_mode":"both","reset_mode":"calendar_month","reset_day":1}`, `{"name":"零额度","port_count":1,"traffic_limit_bytes":"0","accounting_mode":"max","reset_mode":"anniversary"}`} {
		expectCode(t, f.request(t, "POST", "/api/admin/plans", body, nil, true), 400, "validation_error")
	}
	for i, mode := range []string{"both", "max", "upload", "download"} {
		body := fmt.Sprintf(`{"name":"模式%d","port_count":1,"traffic_limit_bytes":"1024","accounting_mode":%q,"reset_mode":"anniversary"}`, i, mode)
		r := f.request(t, "POST", "/api/admin/plans", body, nil, true)
		expectStatus(t, r, 201)
		if i == 0 {
			p := decodeValue[model.Plan](t, r)
			expectCode(t, f.request(t, "POST", "/api/admin/users", jsonBody(t, model.UserInput{Username: "空密码", Password: "        ", PlanID: p.ID}), nil, true), 400, "validation_error")
			expectCode(t, f.request(t, "POST", "/api/admin/users", jsonBody(t, model.UserInput{Username: "无端口用户", Password: "测试密码123", PlanID: p.ID}), nil, true), 409, "port_pool_unset")
		}
	}
	expectCode(t, f.request(t, "PUT", "/api/admin/settings", `{"port_start":20000,"port_end":10000}`, map[string]string{"If-Match": `"1"`}, true), 400, "validation_error")
	expectStatus(t, f.request(t, "PUT", "/api/admin/settings", `{"port_start":20000,"port_end":20010}`, map[string]string{"If-Match": `"1"`}, true), 200)
	expectCode(t, f.request(t, "PUT", "/api/admin/settings", `{"port_start":21000,"port_end":21010}`, map[string]string{"If-Match": `"1"`}, true), 412, "revision_mismatch")
}

func TestAdminControlsUserRealForwardingWithoutUserSession(t *testing.T) {
	admin, user, _, a, _ := provisionAPIUsers(t)
	target := pairedHTTPTarget(t)
	enabled := true
	in := model.Input{Name: "管理员代管", Protocol: "tcp+udp", ListenHost: "127.0.0.1", ListenPort: a.User.PortStart, TargetHost: "127.0.0.1", TargetPort: target, Enabled: &enabled, TrafficEnabled: &enabled}
	response := admin.request(t, "POST", "/api/admin/rules", jsonBody(t, model.AdminRuleCreateInput{Input: in, OwnerID: a.User.ID}), nil, true)
	expectStatus(t, response, 201)
	r := decodeValue[model.Rule](t, response)
	path := "/api/admin/rules/" + r.ID
	verifyHTTPCombinedForwarding(t, in.ListenPort)
	c := combinedHTTPClient(t, in.ListenPort)
	httpTCPExchange(t, c, "管理员代管用户规则")
	enabled = false
	response = admin.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": etag(r.Revision)}, true)
	expectStatus(t, response, 200)
	r = decodeValue[model.Rule](t, response)
	c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("管理员停用没有断开用户连接")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("管理员停用只等待超时")
	}
	enabled = true
	response = admin.request(t, "PUT", path, inputJSON(t, in), map[string]string{"If-Match": etag(r.Revision)}, true)
	expectStatus(t, response, 200)
	r = decodeValue[model.Rule](t, response)
	retry := admin.request(t, "POST", path+"/retry", "", map[string]string{"If-Match": etag(r.Revision)}, true)
	expectStatus(t, retry, 200)
	r = decodeValue[model.Rule](t, retry)
	if r.Owner == nil || r.OwnerID != a.User.ID {
		t.Fatal("重试没有返回用户归属")
	}
	verifyHTTPCombinedForwarding(t, in.ListenPort)
	reset := admin.request(t, "POST", path+"/traffic/reset", "", map[string]string{"If-Match": etag(r.Revision)}, true)
	expectStatus(t, reset, 200)
	r = decodeValue[model.Rule](t, reset)
	visible := user.request(t, "GET", "/api/user/rules", "", nil, true)
	expectStatus(t, visible, 200)
	list := decodeValue[struct {
		Rules []model.Rule `json:"rules"`
	}](t, visible)
	if len(list.Rules) != 1 || list.Rules[0].Revision != r.Revision || list.Rules[0].Owner != nil {
		t.Fatal("用户端未同步管理员操作")
	}
	expectStatus(t, admin.request(t, "DELETE", path, "", map[string]string{"If-Match": etag(r.Revision)}, true), 204)
}
