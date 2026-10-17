package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/auth"
	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

type apiFixture struct {
	server  *httptest.Server
	db      *store.Store
	service *rules.Service
	manager *auth.Manager
	cookie  *http.Cookie
	csrf    string
	once    sync.Once
}

func newAPI(t *testing.T, path string, secure bool) *apiFixture {
	t.Helper()
	if path == "" {
		path = filepath.Join(testutil.TempDir(t), "porter.db")
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnsureAdmin(context.Background(), "管理员"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(http.NotFoundHandler())
	svc, err := rules.New(context.Background(), db, forward.New(forward.Limits{}), ts.Listener.Addr().String())
	if err != nil {
		_ = db.Close()
		ts.Close()
		t.Fatal(err)
	}
	manager := auth.New("管理员", "测试密码")
	manager.SetUsers(db)
	ts.Config.Handler = New(svc, manager, db, secure, http.NotFoundHandler())
	if secure {
		ts.StartTLS()
	} else {
		ts.Start()
	}
	f := &apiFixture{server: ts, db: db, service: svc, manager: manager}
	t.Cleanup(f.close)
	return f
}

func (f *apiFixture) close() {
	f.once.Do(func() { f.server.Close(); f.service.Close(); _ = f.db.Close() })
}

type apiResponse struct {
	status  int
	header  http.Header
	cookies []*http.Cookie
	body    []byte
}

func (f *apiFixture) request(t *testing.T, method, path, body string, headers map[string]string, withAuth bool) apiResponse {
	t.Helper()
	req, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", f.server.URL)
	if withAuth && f.cookie != nil {
		req.AddCookie(f.cookie)
		req.Header.Set("X-CSRF-Token", f.csrf)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	response, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return apiResponse{response.StatusCode, response.Header, response.Cookies(), data}
}

func expectStatus(t *testing.T, response apiResponse, status int) {
	t.Helper()
	if response.status != status {
		t.Fatalf("HTTP 状态应为 %d，实际 %d：%s", status, response.status, response.body)
	}
}
func expectCode(t *testing.T, response apiResponse, status int, code string) {
	t.Helper()
	expectStatus(t, response, status)
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code || body.Error.Message == "" {
		t.Fatalf("错误响应不匹配：%s", response.body)
	}
}

func (f *apiFixture) login(t *testing.T) apiResponse {
	t.Helper()
	response := f.request(t, "POST", "/api/admin/auth/login", `{"username":"管理员","password":"测试密码"}`, nil, false)
	expectStatus(t, response, 200)
	var session auth.Session
	if err := json.Unmarshal(response.body, &session); err != nil {
		t.Fatal(err)
	}
	if session.Username != "管理员" || session.CSRF == "" {
		t.Fatalf("登录响应缺少会话信息：%s", response.body)
	}
	f.csrf = session.CSRF
	for _, c := range response.cookies {
		if c.Name == cookieName {
			f.cookie = c
		}
	}
	if f.cookie == nil {
		t.Fatal("登录没有设置会话 Cookie")
	}
	return response
}

func TestAuthenticationCookiesAndLogoutRevocation(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(strconv.FormatBool(secure), func(t *testing.T) {
			f := newAPI(t, "", secure)
			expectCode(t, f.request(t, "GET", "/api/admin/rules", "", nil, false), 401, "unauthorized")
			expectCode(t, f.request(t, "POST", "/api/admin/auth/login", `{"username":"管理员","password":"错误"}`, nil, false), 401, "credentials_invalid")
			response := f.login(t)
			cookie := f.cookie
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure != secure || cookie.Path != "/api/admin" || cookie.MaxAge != 43200 {
				t.Fatalf("会话 Cookie 属性不正确：%+v", cookie)
			}
			if len(cookie.Value) != 64 || time.Until(cookie.Expires) < 11*time.Hour || time.Until(cookie.Expires) > 12*time.Hour {
				t.Fatal("会话 token 或有效期不正确")
			}
			if bytes.Contains(response.body, []byte(cookie.Value)) || bytes.Contains(response.body, []byte("测试密码")) {
				t.Fatal("登录 JSON 泄露会话 token 或密码")
			}
			if response.header.Get("Cache-Control") != "no-store" || response.header.Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("管理响应缺少安全响应头")
			}
			expectStatus(t, f.request(t, "GET", "/api/admin/auth/session", "", nil, true), 200)
			logout := f.request(t, "POST", "/api/admin/auth/logout", "", nil, true)
			expectStatus(t, logout, 204)
			if len(logout.cookies) != 1 || logout.cookies[0].MaxAge != -1 || !logout.cookies[0].HttpOnly {
				t.Fatal("注销没有清除 Cookie")
			}
			expectCode(t, f.request(t, "GET", "/api/admin/auth/session", "", nil, true), 401, "unauthorized")
			expectCode(t, f.request(t, "POST", "/api/admin/rules", `{}`, nil, true), 401, "unauthorized")
		})
	}
}

func TestReloginRotatesAndRevokesPresentedSession(t *testing.T) {
	f := newAPI(t, "", false)
	f.login(t)
	oldCookie, oldCSRF := f.cookie, f.csrf
	response := f.request(t, "POST", "/api/admin/auth/login", `{"username":"管理员","password":"测试密码"}`, nil, true)
	expectStatus(t, response, 200)
	if len(response.cookies) != 1 || response.cookies[0].Value == oldCookie.Value {
		t.Fatal("重新登录没有轮换 token")
	}
	if _, ok := f.manager.Get(oldCookie.Value); ok {
		t.Fatal("重新登录后旧会话未撤销")
	}
	f.cookie = response.cookies[0]
	var session auth.Session
	_ = json.Unmarshal(response.body, &session)
	f.csrf = session.CSRF
	if f.csrf == oldCSRF {
		t.Fatal("重新登录没有轮换 CSRF token")
	}
	expectStatus(t, f.request(t, "GET", "/api/admin/rules", "", nil, true), 200)
}

func TestLoginRateLimitCannotBeBypassedWithForwardedHeaders(t *testing.T) {
	f := newAPI(t, "", false)
	for i := range 10 {
		response := f.request(t, "POST", "/api/admin/auth/login", `{"username":"管理员","password":"错误"}`, map[string]string{"X-Forwarded-For": "192.0.2." + strconv.Itoa(i+1), "X-Real-IP": "198.51.100." + strconv.Itoa(i+1)}, false)
		expectCode(t, response, 401, "credentials_invalid")
	}
	limited := f.request(t, "POST", "/api/admin/auth/login", `{"username":"管理员","password":"测试密码"}`, nil, false)
	expectCode(t, limited, 429, "login_limited")
	if limited.header.Get("Retry-After") != "60" {
		t.Fatal("限流响应缺少 Retry-After")
	}
	if len(limited.cookies) > 0 {
		t.Fatal("限流请求仍签发会话")
	}
}

func TestOriginAndCSRFRejectBeforeMutation(t *testing.T) {
	f := newAPI(t, "", false)
	for _, origin := range []string{"", "null", "https://example.invalid", "http://attacker.invalid", "http://user@" + strings.TrimPrefix(f.server.URL, "http://"), f.server.URL + "/page", f.server.URL + "?query=1"} {
		t.Run("登录来源"+origin, func(t *testing.T) {
			expectCode(t, f.request(t, "POST", "/api/admin/auth/login", `{"username":"管理员","password":"测试密码"}`, map[string]string{"Origin": origin}, false), 403, "origin_invalid")
		})
	}
	f.login(t)
	for _, headers := range []map[string]string{{"X-CSRF-Token": ""}, {"X-CSRF-Token": "错误"}, {"Origin": ""}, {"Origin": "https://attacker.invalid"}, {"Origin": "https://" + strings.TrimPrefix(f.server.URL, "http://")}} {
		expectCode(t, f.request(t, "POST", "/api/admin/rules", `{}`, headers, true), 403, "csrf_invalid")
		expectCode(t, f.request(t, "POST", "/api/admin/auth/logout", "", headers, true), 403, "csrf_invalid")
	}
	if len(f.service.List()) != 0 {
		t.Fatal("CSRF 拒绝请求写入了规则")
	}
	expectStatus(t, f.request(t, "GET", "/api/admin/auth/session", "", nil, true), 200)
}

func TestStrictJSONAndValidationResponses(t *testing.T) {
	f := newAPI(t, "", false)
	for _, input := range []struct{ body, contentType, code string }{
		{`{"username":"管理员","password":"测试密码","unexpected":true}`, "application/json", "invalid_json"},
		{`{"username":"管理员","password":"测试密码"} {}`, "application/json", "invalid_json"},
		{`[1,2]`, "application/json", "invalid_json"},
		{`{"username":1}`, "application/json", "invalid_json"},
		{`{`, "application/json", "invalid_json"},
		{`{"username":"管理员","password":"测试密码"}`, "text/plain", "json_required"},
		{`{"username":"` + strings.Repeat("a", 65536) + `"}`, "application/json", "invalid_json"},
	} {
		expectCode(t, f.request(t, "POST", "/api/admin/auth/login", input.body, map[string]string{"Content-Type": input.contentType}, false), 400, input.code)
	}
	f.login(t)
	for _, body := range []string{`{"id":"自定义ID"}`, `{"enabled":"true"}`, `{"listen_port":1.5}`, `{} {}`, `[]`} {
		expectCode(t, f.request(t, "POST", "/api/admin/rules", body, nil, true), 400, "invalid_json")
	}
	response := f.request(t, "POST", "/api/admin/rules", `{"name":"规则","protocol":"tcp","listen_host":"127.0.0.1","listen_port":0,"target_host":"127.0.0.1","target_port":1}`, nil, true)
	expectCode(t, response, 400, "validation_error")
	var body struct {
		Error struct {
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	_ = json.Unmarshal(response.body, &body)
	if body.Error.Fields["listen_port"] == "" {
		t.Fatal("表单错误没有指出非法端口")
	}
	if len(f.service.List()) != 0 {
		t.Fatal("无效 JSON 请求创建了规则")
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func inputJSON(t *testing.T, input model.Input) string {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func responseRule(t *testing.T, response apiResponse) model.Rule {
	t.Helper()
	var rule model.Rule
	if err := json.Unmarshal(response.body, &rule); err != nil {
		t.Fatal(err)
	}
	return rule
}
func ruleList(t *testing.T, f *apiFixture) []model.Rule {
	t.Helper()
	response := f.request(t, "GET", "/api/admin/rules", "", nil, true)
	expectStatus(t, response, 200)
	var body struct {
		Rules []model.Rule `json:"rules"`
	}
	if err := json.Unmarshal(response.body, &body); err != nil {
		t.Fatal(err)
	}
	return body.Rules
}

func TestRuleLifecycleRevisionAndSocketRelease(t *testing.T) {
	f := newAPI(t, "", false)
	f.login(t)
	port := freeTCPPort(t)
	enabled := true
	input := model.Input{Name: "测试入口", Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: port, TargetHost: "127.0.0.1", TargetPort: 1, Enabled: &enabled}
	created := f.request(t, "POST", "/api/admin/rules", inputJSON(t, input), nil, true)
	expectStatus(t, created, 201)
	rule := responseRule(t, created)
	if rule.ID == "" || rule.Revision != 1 || rule.Status != "running" || created.header.Get("ETag") != `"1"` {
		t.Fatalf("创建规则契约错误：%s", created.body)
	}
	if list := ruleList(t, f); len(list) != 1 || list[0].ID != rule.ID {
		t.Fatalf("列表未返回创建规则：%+v", list)
	}
	path := "/api/admin/rules/" + rule.ID
	for _, revision := range []string{"", "1", `"0"`, `"invalid"`} {
		response := f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": revision}, true)
		expectStatus(t, response, 400)
	}
	expectCode(t, f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": `"2"`}, true), 412, "revision_mismatch")
	input.Name = "新名称"
	input.TargetPort = 2
	updated := f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": `"1"`}, true)
	expectStatus(t, updated, 200)
	rule = responseRule(t, updated)
	if rule.Name != "新名称" || rule.TargetPort != 2 || rule.Revision != 2 || updated.header.Get("ETag") != `"2"` {
		t.Fatalf("编辑规则未生效：%s", updated.body)
	}
	input.Enabled = nil
	expectCode(t, f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": `"2"`}, true), 400, "validation_error")
	enabled = false
	input.Enabled = &enabled
	disabled := f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": `"2"`}, true)
	expectStatus(t, disabled, 200)
	rule = responseRule(t, disabled)
	if rule.Enabled || rule.Status != "disabled" || rule.Revision != 3 {
		t.Fatalf("停用状态错误：%s", disabled.body)
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("停用后监听端口未释放：%v", err)
	}
	_ = ln.Close()
	expectCode(t, f.request(t, "POST", path+"/retry", "", map[string]string{"If-Match": `"3"`}, true), 409, "disabled")
	enabled = true
	reenabled := f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": `"3"`}, true)
	expectStatus(t, reenabled, 200)
	if r := responseRule(t, reenabled); r.Status != "running" || r.Revision != 4 {
		t.Fatalf("重新启用状态错误：%s", reenabled.body)
	}
	expectCode(t, f.request(t, "DELETE", path, "", map[string]string{"If-Match": `"3"`}, true), 412, "revision_mismatch")
	expectStatus(t, f.request(t, "DELETE", path, "", map[string]string{"If-Match": `"4"`}, true), 204)
	if len(ruleList(t, f)) != 0 {
		t.Fatal("删除后列表仍有规则")
	}
	ln, err = net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("删除后监听端口未释放：%v", err)
	}
	_ = ln.Close()
	expectCode(t, f.request(t, "DELETE", path, "", map[string]string{"If-Match": `"4"`}, true), 404, "not_found")
}

func TestConcurrentUpdatesCannotOverwriteSameRevision(t *testing.T) {
	f := newAPI(t, "", false)
	f.login(t)
	enabled := false
	input := model.Input{Name: "原名称", Protocol: "udp", ListenHost: "127.0.0.1", ListenPort: 12345, TargetHost: "127.0.0.1", TargetPort: 1, Enabled: &enabled}
	created := f.request(t, "POST", "/api/admin/rules", inputJSON(t, input), nil, true)
	expectStatus(t, created, 201)
	rule := responseRule(t, created)
	start := make(chan struct{})
	responses := make(chan apiResponse, 2)
	for _, name := range []string{"页面甲", "页面乙"} {
		go func() {
			response := apiResponse{}
			defer func() { responses <- response }()
			candidate := input
			candidate.Name = name
			<-start
			response = f.request(t, "PUT", "/api/admin/rules/"+rule.ID, inputJSON(t, candidate), map[string]string{"If-Match": `"1"`}, true)
		}()
	}
	close(start)
	success, conflict := 0, 0
	winner := ""
	for range 2 {
		response := <-responses
		switch response.status {
		case 200:
			success++
			winner = responseRule(t, response).Name
		case 412:
			expectCode(t, response, 412, "revision_mismatch")
			conflict++
		default:
			t.Fatalf("并发编辑返回意外状态：%d %s", response.status, response.body)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("相同 revision 的并发编辑结果：成功 %d，冲突 %d", success, conflict)
	}
	list := ruleList(t, f)
	if len(list) != 1 || list[0].Revision != 2 || list[0].Name != winner {
		t.Fatalf("失败写操作覆盖了成功结果：%+v", list)
	}
}

func TestRestartPersistsRulesAndInvalidatesSessions(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "porter.db")
	f := newAPI(t, path, false)
	f.login(t)
	enabled := false
	input := model.Input{Name: "重启保留", Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: 12345, TargetHost: "example.com", TargetPort: 443, Enabled: &enabled}
	created := f.request(t, "POST", "/api/admin/rules", inputJSON(t, input), nil, true)
	expectStatus(t, created, 201)
	rule := responseRule(t, created)
	cookie, csrf := f.cookie, f.csrf
	f.close()
	restarted := newAPI(t, path, false)
	restarted.cookie = cookie
	restarted.csrf = csrf
	expectCode(t, restarted.request(t, "GET", "/api/admin/rules", "", nil, true), 401, "unauthorized")
	restarted.login(t)
	list := ruleList(t, restarted)
	if len(list) != 1 || list[0].ID != rule.ID || list[0].Name != "重启保留" || list[0].Status != "disabled" || list[0].Revision != 1 {
		t.Fatalf("重启后规则未正确恢复：%+v", list)
	}
}

func TestHealthReadinessAndUnknownAPIRoute(t *testing.T) {
	f := newAPI(t, "", false)
	health := f.request(t, "GET", "/healthz", "", nil, false)
	expectStatus(t, health, 200)
	if strings.TrimSpace(string(health.body)) != `{"status":"ok"}` {
		t.Fatalf("健康接口包含多余信息：%s", health.body)
	}
	expectCode(t, f.request(t, "GET", "/api/unknown", "", nil, false), 404, "not_found")
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	unavailable := f.request(t, "GET", "/healthz", "", nil, false)
	expectStatus(t, unavailable, 503)
	if strings.TrimSpace(string(unavailable.body)) != `{"status":"unavailable"}` {
		t.Fatalf("健康错误接口泄露内部信息：%s", unavailable.body)
	}
}
