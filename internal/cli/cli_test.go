package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/lwl0820/porter/internal/controlapi"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/testutil"
)

func TestLanguageScanDoesNotConsumeFieldValuesAsFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		lang string
		json bool
		bad  bool
	}{
		{[]string{"list", "--lang", "en"}, "en", false, false},
		{[]string{"--lang=zh-CN", "get", "id", "--json"}, "zh-CN", true, false},
		{[]string{"add", "--name", "--lang=en", "--json"}, "", true, false},
		{[]string{"add", "--target-host=--lang=en", "--lang", "zh-CN"}, "zh-CN", false, false},
		{[]string{"--lang", "fr"}, "", false, true},
		{[]string{"--lang", "en", "--lang", "en"}, "en", false, true},
		{[]string{"--lang"}, "", false, true},
	} {
		lang, j, err := languageOption(tc.args)
		if lang != tc.lang || j != tc.json || (err != nil) != tc.bad {
			t.Fatalf("预扫描 %v = %s %v %v", tc.args, lang, j, err)
		}
	}
}
func TestParameterContracts(t *testing.T) {
	for _, args := range [][]string{
		{"add", "--lang", "en"}, {"update", "id", "--lang=en"},
		{"update", "id", "--owner-id", "other"}, {"get"}, {"delete", "id", "extra"},
		{"add", "--enabled", "false"}, {"update", "id", "--enabled=0"}, {"list", "--unknown"},
		{"update", "id", "--name", "new", "--revision", "0"}, {"list", "--listen-port", "1"},
		{"update", "id", "--listen-port", "65536"}, {"update", "id", "--target-port", "a"},
		{"get", "../id"}, {"get", "?id"}, {"--lang", "en", "--lang", "en"},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("非法参数通过：%v", args)
		}
	}
	for _, args := range [][]string{
		{"--lang", "en", "list", "--json"}, {"get", "id", "--lang=en"},
		{"update", "id", "--enabled=false", "--traffic-enabled=false", "--revision=2"},
		{"delete", "id", "--revision", "2", "--socket", "/custom/control.sock"},
		{"add", "--name", "规则", "--protocol", "tcp+udp", "--listen-host", "::", "--listen-port", "8001", "--target-host", "example.com", "--target-port", "80", "--enabled=false", "--traffic-enabled=true", "--owner-id", "user"},
	} {
		if _, err := parse(args); err != nil {
			t.Errorf("合法参数失败：%v %v", args, err)
		}
	}
}
func TestHelpAndUsageBeforePermissionsWithoutService(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN"} {
		var out, stderr bytes.Buffer
		rc := Run(context.Background(), []string{"rules", "--help", "--lang=" + lang}, &out, &stderr)
		if rc != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), "--revision") {
			t.Fatalf("帮助失败：%d %s %s", rc, &out, &stderr)
		}
		out.Reset()
		stderr.Reset()
		rc = Run(context.Background(), []string{"rules", "update", "id", "--lang", lang, "--json"}, &out, &stderr)
		if rc != 2 || out.Len() != 0 {
			t.Fatal("参数错误出口错误")
		}
		var got struct{ Error controlapi.Error }
		if err := json.Unmarshal(stderr.Bytes(), &got); err != nil || got.Error.Code != "no_changes" {
			t.Fatalf("错误JSON无效：%s", &stderr)
		}
		if lang == "en" {
			assertNoChinese(t, got.Error.Message)
		}
	}
}
func TestErrorTranslationsAreSelectedAndStructured(t *testing.T) {
	codes := []string{"usage_error", "no_changes", "menu_json", "terminal_required", "native_only", "root_required", "forbidden", "service_unavailable", "request_timeout", "invalid_response", "validation_error", "not_found", "revision_mismatch", "revision_required", "revision_invalid", "rule_conflict", "listen_conflict", "admin_conflict", "port_reserved", "quota_exceeded", "rule_limit", "forward_loop", "database_error", "shutting_down", "json_required", "invalid_json", "internal_error", "listen_failed", "quota_reset_pending", "future_error"}
	for _, lang := range []string{"en", "zh-CN"} {
		for _, code := range codes {
			var stderr bytes.Buffer
			r := runner{lang: lang, err: &stderr}
			detail := "不应输出的动态中文"
			if code == "usage_error" {
				detail = "--flag"
			}
			r.fail(&controlapi.Error{Code: code, Message: detail, Fields: map[string]string{"listen_port": "动态中文", "target_host": "动态中文", "future": "动态中文"}}, true)
			var got struct{ Error controlapi.Error }
			if err := json.Unmarshal(stderr.Bytes(), &got); err != nil || got.Error.Code != code {
				t.Fatal("错误包络不可解析")
			}
			if strings.Contains(got.Error.Message, "动态中文") {
				t.Fatal("泄漏动态服务文案")
			}
			if lang == "en" {
				assertNoChinese(t, got.Error.Message)
				for _, message := range got.Error.Fields {
					assertNoChinese(t, message)
				}
			}
		}
	}
}
func assertNoChinese(t *testing.T, value string) {
	t.Helper()
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			t.Fatalf("英文混入中文：%s", value)
		}
	}
}

type fakeAPI struct {
	rule                            model.Rule
	exists                          bool
	creates, updates, deletes, gets int
	afterGet                        func()
	fail                            error
}

func (f *fakeAPI) List(context.Context, string) ([]model.Rule, error) {
	if !f.exists {
		return []model.Rule{}, f.fail
	}
	return []model.Rule{f.rule}, f.fail
}
func (f *fakeAPI) Get(context.Context, string) (model.Rule, error) {
	f.gets++
	if !f.exists {
		return model.Rule{}, cliError("not_found")
	}
	old := f.rule
	if f.afterGet != nil {
		f.afterGet()
	}
	return old, f.fail
}
func (f *fakeAPI) Create(_ context.Context, in model.AdminRuleCreateInput) (model.Rule, error) {
	f.creates++
	if f.fail != nil {
		return model.Rule{}, f.fail
	}
	f.rule = ruleFrom(in.Input)
	f.rule.OwnerID = in.OwnerID
	if f.rule.OwnerID == "" {
		f.rule.OwnerID = model.AdminID
	}
	f.exists = true
	return f.rule, nil
}
func (f *fakeAPI) Update(_ context.Context, id string, rev int64, in model.Input) (model.Rule, error) {
	f.updates++
	if rev != f.rule.Revision {
		return model.Rule{}, cliError("revision_mismatch")
	}
	if f.fail != nil {
		return model.Rule{}, f.fail
	}
	v := ruleFrom(in)
	v.ID = id
	v.Revision = rev + 1
	v.OwnerID = f.rule.OwnerID
	f.rule = v
	return v, nil
}
func (f *fakeAPI) Delete(_ context.Context, _ string, rev int64) error {
	f.deletes++
	if rev != f.rule.Revision {
		return cliError("revision_mismatch")
	}
	if f.fail != nil {
		return f.fail
	}
	f.exists = false
	return nil
}
func ruleFrom(in model.Input) model.Rule {
	enabled, traffic := true, false
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.TrafficEnabled != nil {
		traffic = *in.TrafficEnabled
	}
	return model.Rule{ID: "stable-id", Name: in.Name, Protocol: in.Protocol, ListenHost: in.ListenHost, ListenPort: in.ListenPort, TargetHost: in.TargetHost, TargetPort: in.TargetPort, Enabled: enabled, TrafficEnabled: traffic, OwnerID: model.AdminID, Revision: 1, Status: "running"}
}
func testRule() model.Rule {
	return model.Rule{ID: "stable-id", Name: "before", Protocol: "tcp+udp", ListenHost: "::", ListenPort: 8010, TargetHost: "example.com", TargetPort: 80, Enabled: false, TrafficEnabled: true, OwnerID: "user-id", Revision: 8}
}
func TestReadMergeWriteAndExplicitRevision(t *testing.T) {
	ctx := context.Background()
	f := &fakeAPI{rule: testRule(), exists: true}
	r := runner{ctx: ctx, api: f}
	o, err := parse([]string{"update", "stable-id", "--name", "after"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := r.execute(o)
	if err != nil {
		t.Fatal(err)
	}
	got := value.(model.Rule)
	if got.Name != "after" || got.Enabled || !got.TrafficEnabled || got.OwnerID != "user-id" || got.ListenHost != "::" || got.Revision != 9 {
		t.Fatalf("部分修改破坏其他字段：%+v", got)
	}
	f.afterGet = func() { f.rule.Revision++ }
	_, err = r.execute(o)
	var e *controlapi.Error
	if !errors.As(err, &e) || e.Code != "revision_mismatch" {
		t.Fatal("读后并发变更未拒绝")
	}
	f.afterGet = nil
	o.revision = 1
	_, err = r.execute(o)
	if !errors.As(err, &e) || e.Code != "revision_mismatch" {
		t.Fatal("显式版本被替换")
	}
	o.action = "delete"
	_, err = r.execute(o)
	if !errors.As(err, &e) || e.Code != "revision_mismatch" {
		t.Fatal("删除替换了显式版本")
	}
}
func scriptedRunner(t *testing.T, lang, input string, api *fakeAPI) (*runner, func() string) {
	t.Helper()
	file, err := os.Create(filepath.Join(testutil.TempDir(t), "terminal.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	tty := &terminal{file: file, reader: bufio.NewReader(strings.NewReader(input)), ctx: context.Background()}
	r := &runner{ctx: context.Background(), lang: lang, out: file, err: file, api: api, tty: tty}
	return r, func() string {
		data, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}
func TestBilingualInteractiveCRUDAndCancellation(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		t.Run(lang, func(t *testing.T) {
			// 创建→列表→无效选择后详情→修改留空保留→取消删除→确认删除→退出。
			script := "1\nmenu-rule\ntcp+udp\n127.0.0.1\n8123\nexample.com\n80\ntrue\nfalse\n\nyes\n2\n3\n99\n1\n4\n1\nrenamed\n\n\n\n\n\n\n\nyes\n5\n1\n\n5\n1\nyes\n0\n"
			f := &fakeAPI{}
			r, output := scriptedRunner(t, lang, script, f)
			if err := r.menu(); err != nil {
				t.Fatal(err)
			}
			if f.creates != 1 || f.updates != 1 || f.deletes != 1 || f.exists {
				t.Fatalf("交互操作错误：%+v\n%s", f, output())
			}
			if f.rule.Name != "renamed" || f.rule.Protocol != "tcp+udp" || f.rule.ListenPort != 8123 || !f.rule.Enabled || f.rule.TrafficEnabled {
				t.Fatalf("修改未保持字段：%+v", f.rule)
			}
			if lang == "en" {
				assertNoChinese(t, output())
			}
		})
	}
}
func TestMenuPendingInputsNeverCommitAfterCancelOrEOF(t *testing.T) {
	for _, script := range []string{"1\n/cancel\n0\n", "1\npending\ntcp\n127.0.0.1\n8123\nexample.com\n80\ntrue\nfalse\n\nyes", "1\npending\n"} {
		f := &fakeAPI{}
		r, _ := scriptedRunner(t, "en", script, f)
		err := r.menu()
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if f.creates+f.updates+f.deletes != 0 {
			t.Fatal("取消或EOF提交了变更")
		}
	}
	for _, script := range []string{"4\n1\n/cancel\n0\n", "4\n1\nnew\n", "5\n1\n"} {
		f := &fakeAPI{rule: testRule(), exists: true}
		r, _ := scriptedRunner(t, "en", script, f)
		_ = r.menu()
		if f.updates+f.deletes != 0 {
			t.Fatal("编辑或删除EOF提交了变更")
		}
	}
}
func TestMenuUsesDisplayedRevision(t *testing.T) {
	for _, action := range []string{"4", "5"} {
		f := &fakeAPI{rule: testRule(), exists: true}
		f.afterGet = func() { f.rule.Revision++ }
		script := action + "\n1\n"
		if action == "4" {
			script += "new\n\n\n\n\n\n\n\n"
		}
		script += "yes\n0\n"
		r, out := scriptedRunner(t, "en", script, f)
		if err := r.menu(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out(), "revision_mismatch") || !f.exists || f.rule.Name != "before" {
			t.Fatalf("确认时覆盖并发变更：%s", out())
		}
	}
}
func TestLanguageSelectionDefaultsEnglishAndCancelsWithoutSaving(t *testing.T) {
	for _, tc := range []struct {
		input, lang string
		ended       bool
	}{{"\n", "en", false}, {"invalid\n2\n", "zh-CN", false}, {"1\n", "en", false}, {"en\n", "en", false}, {"zh-CN\n", "zh-CN", false}, {"q\n", "", true}, {"2", "", true}} {
		r, out := scriptedRunner(t, "", tc.input, &fakeAPI{})
		lang, err := r.tty.language()
		if lang != tc.lang || (err != nil) != tc.ended {
			t.Fatalf("语言选择错误：%s %v", lang, err)
		}
		if tc.input == "invalid\n2\n" && strings.Count(out(), "Language / 语言") != 2 {
			t.Fatal("无效输入未重新提示")
		}
	}
}

func TestTerminalCancellationAndExactCounters(t *testing.T) {
	r, _ := scriptedRunner(t, "en", "", &fakeAPI{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.tty.ctx = ctx
	if _, err := r.tty.read("prompt"); !errors.Is(err, io.EOF) {
		t.Fatal("中断未退出")
	}
	var out bytes.Buffer
	value := testRule()
	value.Traffic.UploadBytes = 9007199254740993
	value.CreatedAt = time.Now()
	value.UpdatedAt = value.CreatedAt
	r.detail(&out, value)
	if !strings.Contains(out.String(), "9007199254740993 B") {
		t.Fatal("计数丢失精度")
	}
}

func TestUnknownTopLevelCommandAndLocalizedRuleJSON(t *testing.T) {
	var out, stderr bytes.Buffer
	rc := Run(context.Background(), []string{"list", "--lang", "en", "--json"}, &out, &stderr)
	if rc != 2 || !strings.Contains(stderr.String(), "usage_error") || out.Len() != 0 {
		t.Fatal("未限定 rules 命令入口")
	}
	r := runner{lang: "en"}
	rule := testRule()
	rule.LastError = "动态中文"
	rule.LastErrorCode = "listen_failed"
	value := r.localizeResult(controlapi.RuleList{Rules: []model.Rule{rule}}).(controlapi.RuleList)
	assertNoChinese(t, value.Rules[0].LastError)
	if value.Rules[0].LastErrorCode != "listen_failed" || value.Rules[0].Status != rule.Status {
		t.Fatal("翻译修改了稳定标识")
	}
}
