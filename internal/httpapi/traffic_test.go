package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/lwl0820/porter/internal/model"
)

func TestTrafficInputResetAuthenticationAndRevision(t *testing.T) {
	f := newAPI(t, "", false)
	expectCode(t, f.request(t, "POST", "/api/admin/rules/missing/traffic/reset", "", nil, false), 401, "unauthorized")
	f.login(t)
	disabled := false
	input := model.Input{Name: "流量规则", Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: freeTCPPort(t), TargetHost: "example.com", TargetPort: 443, Enabled: &disabled}
	created := f.request(t, "POST", "/api/admin/rules", inputJSON(t, input), nil, true)
	expectStatus(t, created, 201)
	r := responseRule(t, created)
	if r.TrafficEnabled || r.Traffic.UploadBytes != 0 {
		t.Fatal("省略开关未默认关闭")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(created.body, &raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw["traffic"]), `"upload_bytes":"0"`) {
		t.Fatalf("累计量未输出字符串：%s", created.body)
	}
	path := "/api/admin/rules/" + r.ID
	enabled := true
	input.TrafficEnabled = &enabled
	updated := f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": `"1"`}, true)
	expectStatus(t, updated, 200)
	if !responseRule(t, updated).TrafficEnabled {
		t.Fatal("未开启统计")
	}
	input.TrafficEnabled = nil
	updated = f.request(t, "PUT", path, inputJSON(t, input), map[string]string{"If-Match": `"2"`}, true)
	expectStatus(t, updated, 200)
	if !responseRule(t, updated).TrafficEnabled {
		t.Fatal("显式null未保留开关")
	}
	for _, extra := range []string{`"traffic_enabled":"true"`, `"traffic_enabled":1`, `"traffic":{"upload_bytes":"100"}`, `"upload_bytes":"100"`} {
		body := strings.TrimSuffix(inputJSON(t, input), "}")
		body = strings.ReplaceAll(body, `,"traffic_enabled":null`, "") + "," + extra + "}"
		expectCode(t, f.request(t, "PUT", path, body, map[string]string{"If-Match": `"3"`}, true), 400, "invalid_json")
	}
	resetPath := path + "/traffic/reset"
	headers := map[string]string{"If-Match": `"3"`}
	expectCode(t, f.request(t, "POST", resetPath, "", map[string]string{"If-Match": `"3"`, "X-CSRF-Token": "invalid"}, true), 403, "csrf_invalid")
	expectCode(t, f.request(t, "POST", resetPath, "", map[string]string{"If-Match": `"3"`, "Origin": "http://foreign.example"}, true), 403, "csrf_invalid")
	expectCode(t, f.request(t, "POST", resetPath, "", nil, true), 400, "revision_required")
	reset := f.request(t, "POST", resetPath, "", headers, true)
	expectStatus(t, reset, 200)
	r = responseRule(t, reset)
	if r.Revision != 4 || !r.TrafficEnabled || r.Enabled || reset.header.Get("ETag") != `"4"` {
		t.Fatalf("重置改变了配置或未更新版本：%s", reset.body)
	}
	expectCode(t, f.request(t, "POST", resetPath, "", headers, true), 412, "revision_mismatch")
	expectCode(t, f.request(t, "POST", "/api/admin/rules/missing/traffic/reset", "", headers, true), 404, "not_found")
}

func TestTrafficResetAndConfigConcurrentRevision(t *testing.T) {
	f := newAPI(t, "", false)
	f.login(t)
	disabled := false
	input := model.Input{Name: "并发规则", Protocol: "tcp", ListenHost: "127.0.0.1", ListenPort: freeTCPPort(t), TargetHost: "example.com", TargetPort: 443, Enabled: &disabled}
	created := f.request(t, "POST", "/api/admin/rules", inputJSON(t, input), nil, true)
	expectStatus(t, created, 201)
	r := responseRule(t, created)
	path := "/api/admin/rules/" + r.ID
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	body := inputJSON(t, input)
	for _, operation := range []struct{ method, path, body string }{{http.MethodPut, path, body}, {http.MethodPost, path + "/traffic/reset", ""}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := f.request(t, operation.method, operation.path, operation.body, map[string]string{"If-Match": `"1"`}, true)
			statuses <- response.status
		}()
	}
	wg.Wait()
	close(statuses)
	results := map[int]int{}
	for status := range statuses {
		results[status]++
	}
	if results[200] != 1 || results[412] != 1 {
		t.Fatalf("同版本更新与重置未互斥：%v", results)
	}
}
