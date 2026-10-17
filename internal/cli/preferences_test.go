package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type memoryPreferences struct {
	language         string
	readErr, saveErr error
	reads, saves     int
}

func (p *memoryPreferences) Read() (string, error) { p.reads++; return p.language, p.readErr }
func (p *memoryPreferences) Save(_ context.Context, lang string, overwrite bool) (string, error) {
	p.saves++
	if p.saveErr != nil {
		return "", p.saveErr
	}
	if p.language == "" || overwrite {
		p.language = lang
	}
	return p.language, nil
}

func TestFirstLanguagePersistsAndMenuChangesIt(t *testing.T) {
	for _, tc := range []struct{ input, lang string }{{"\n0\n", "en"}, {"2\n0\n", "zh-CN"}, {"1\n6\n2\n0\n", "zh-CN"}} {
		r, _ := scriptedRunner(t, "", tc.input, &fakeAPI{})
		p := &memoryPreferences{}
		var out, stderr bytes.Buffer
		rc := run(context.Background(), nil, &out, &stderr, p, func(context.Context) (*terminal, error) { return r.tty, nil }, true, 0)
		if rc != 0 || p.language != tc.lang || p.saves < 1 || stderr.Len() != 0 || out.Len() != 0 {
			t.Fatalf("首次/切换保存失败：%d %+v %s %s", rc, p, &out, &stderr)
		}
	}
}
func TestSavedLanguageReusedAndExplicitOverrideIsTransient(t *testing.T) {
	for _, prefix := range [][]string{nil, {"rules"}} {
		for _, explicit := range []bool{false, true} {
			p := &memoryPreferences{language: "zh-CN"}
			args := append(append([]string{}, prefix...), "--help")
			if explicit {
				args = append(args, "--lang=en")
			}
			var out, stderr bytes.Buffer
			rc := run(context.Background(), args, &out, &stderr, p, func(context.Context) (*terminal, error) { t.Fatal("有语言设置仍打开终端"); return nil, nil }, true, 0)
			if rc != 0 || p.saves != 0 || p.language != "zh-CN" || stderr.Len() != 0 {
				t.Fatalf("复用设置失败：%d %+v %s", rc, p, &stderr)
			}
			if explicit {
				assertNoChinese(t, out.String())
				if p.reads != 0 {
					t.Fatal("显式语言仍读取配置")
				}
			} else if !strings.Contains(out.String(), "用法") {
				t.Fatal("未采用保存的中文")
			}
		}
	}
}
func TestLanguageCancellationReadonlyAndSaveFailure(t *testing.T) {
	for _, input := range []string{"q\n", "2", ""} {
		r, _ := scriptedRunner(t, "", input, &fakeAPI{})
		p := &memoryPreferences{}
		var out bytes.Buffer
		rc := run(context.Background(), nil, &out, &out, p, func(context.Context) (*terminal, error) { return r.tty, nil }, true, 0)
		if rc != 0 || p.saves != 0 {
			t.Fatalf("取消或EOF保存：%d %+v", rc, p)
		}
	}
	for _, tc := range []struct {
		available bool
		uid       int
		args      []string
	}{{false, 0, nil}, {true, 1000, nil}, {true, 0, []string{"--help"}}} {
		r, _ := scriptedRunner(t, "", "\n0\n", &fakeAPI{})
		p := &memoryPreferences{}
		var out, stderr bytes.Buffer
		_ = run(context.Background(), tc.args, &out, &stderr, p, func(context.Context) (*terminal, error) { return r.tty, nil }, tc.available, tc.uid)
		if p.saves != 0 || !strings.Contains(stderr.String(), "preference_readonly") {
			t.Fatalf("只读调用尝试保存：%+v %s", tc, &stderr)
		}
	}
	r, _ := scriptedRunner(t, "", "2\n0\n", &fakeAPI{})
	p := &memoryPreferences{saveErr: errors.New("模拟写失败")}
	var out, stderr bytes.Buffer
	if rc := run(context.Background(), nil, &out, &stderr, p, func(context.Context) (*terminal, error) { return r.tty, nil }, true, 0); rc != 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "未能保存") {
		t.Fatalf("保存失败未继续或未警告：%d %s", rc, &stderr)
	}
}
func TestNoTerminalAndInvalidConfigurationAreVisible(t *testing.T) {
	for _, p := range []*memoryPreferences{{}, {readErr: errors.New("非法配置")}} {
		var out, stderr bytes.Buffer
		rc := run(context.Background(), []string{"rules", "list", "--json"}, &out, &stderr, p, func(context.Context) (*terminal, error) { return nil, errors.New("无终端") }, true, 0)
		if rc != 2 || out.Len() != 0 || p.saves != 0 {
			t.Fatalf("错误出口不正确：%d %s", rc, &stderr)
		}
		if p.readErr != nil && !strings.Contains(stderr.String(), "preference_invalid") {
			t.Fatal("损坏设置被当作首次运行")
		}
		if p.readErr == nil && !strings.Contains(stderr.String(), "language_required") {
			t.Fatal("无终端未要求显式语言")
		}
	}
}

func TestExplicitLanguageBypassesInvalidPreference(t *testing.T) {
	p := &memoryPreferences{readErr: errors.New("配置损坏")}
	var out, stderr bytes.Buffer
	rc := run(context.Background(), []string{"rules", "--help", "--lang=en"}, &out, &stderr, p, func(context.Context) (*terminal, error) {
		t.Fatal("显式语言仍要求终端")
		return nil, nil
	}, true, 0)
	if rc != 0 || p.reads != 0 || p.saves != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("显式语言无法绕过损坏偏好：%d %+v %s", rc, p, &stderr)
	}
}

func TestHelpLikeFieldValueDoesNotDisableFirstPreferenceSave(t *testing.T) {
	for _, name := range []string{"--help", "-h"} {
		r, _ := scriptedRunner(t, "", "\n", &fakeAPI{})
		p := &memoryPreferences{}
		var out, stderr bytes.Buffer
		// 不完整的业务参数在语言保存后直接报错；字段值不能触发只读帮助。
		rc := run(context.Background(), []string{"rules", "add", "--name", name}, &out, &stderr, p, func(context.Context) (*terminal, error) {
			return r.tty, nil
		}, true, 0)
		if rc != 2 || p.saves != 1 || p.language != "en" || !strings.Contains(stderr.String(), "usage_error") || strings.Contains(stderr.String(), "preference_readonly") {
			t.Fatalf("字段值被当作帮助参数：%s %d %+v %s", name, rc, p, &stderr)
		}
	}
}

func TestDecodeLanguagePreference(t *testing.T) {
	for _, value := range []string{"", `{}`, `null`, `{"schema":2,"language":"en"}`, `{"schema":1,"language":"fr"}`, `{"schema":1,"language":"en","other":true}`, `{"schema":1,"language":"en","language":"zh-CN"}`, `{"schema":1,"language":"en"} {}`, strings.Repeat(" ", 4097) + `{}`} {
		if _, err := decodePreference([]byte(value)); err == nil {
			t.Fatalf("非法配置通过：%q", value)
		}
	}
	for _, lang := range []string{"en", "zh-CN"} {
		got, err := decodePreference([]byte(`{"schema":1,"language":"` + lang + `"}`))
		if got != lang || err != nil {
			t.Fatal("合法配置失败")
		}
	}
}
