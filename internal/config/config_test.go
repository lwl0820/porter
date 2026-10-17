package config

import (
	"testing"
	"time"
)

func TestTrafficFlushInterval(t *testing.T) {
	t.Setenv("PORTER_ADMIN_USERNAME", "管理员")
	t.Setenv("PORTER_ADMIN_PASSWORD", "密码")
	t.Setenv("PORTER_HTTP_ADDR", "127.0.0.1:8080")
	for _, test := range []struct {
		value string
		want  time.Duration
		bad   bool
	}{
		{"", 5 * time.Second, false}, {"1ms", time.Millisecond, false}, {"1s", time.Second, false}, {"30s", 30 * time.Second, false}, {"24h", 24 * time.Hour, false},
		{"0s", 0, true}, {"-1s", 0, true}, {"1us", 0, true}, {"24h1ms", 0, true}, {"五秒", 0, true}, {"5", 0, true},
	} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("PORTER_TRAFFIC_FLUSH_INTERVAL", test.value)
			got, err := Load()
			if test.bad {
				if err == nil {
					t.Fatal("非法保存间隔未拒绝")
				}
				return
			}
			if err != nil || got.TrafficFlushInterval != test.want {
				t.Fatalf("保存间隔：%v %v", got.TrafficFlushInterval, err)
			}
		})
	}
}

func TestManagementModes(t *testing.T) {
	for _, test := range []struct {
		name, web, cli        string
		wantWeb, wantCLI, bad bool
	}{
		{"旧配置", "", "", true, true, false},
		{"双入口", "true", "true", true, true, false},
		{"仅Web", "true", "false", true, false, false},
		{"仅CLI", "false", "true", false, true, false},
		{"全关", "false", "false", false, false, true},
		{"Web非法", "yes", "true", false, false, true},
		{"CLI非法", "true", "no", false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PORTER_WEB_ENABLED", test.web)
			t.Setenv("PORTER_CLI_ENABLED", test.cli)
			t.Setenv("PORTER_ADMIN_USERNAME", "管理员")
			t.Setenv("PORTER_ADMIN_PASSWORD", "密码")
			t.Setenv("PORTER_HTTP_ADDR", "127.0.0.1:8080")
			got, err := Load()
			if test.bad {
				if err == nil {
					t.Fatal("非法入口组合未拒绝")
				}
				return
			}
			if err != nil || got.WebEnabled != test.wantWeb || got.CLIEnabled != test.wantCLI {
				t.Fatalf("组合读取错误：%+v %v", got, err)
			}
		})
	}
}

func TestWebOnlyFieldsAreConditional(t *testing.T) {
	for _, key := range []string{"PORTER_ADMIN_USERNAME", "PORTER_ADMIN_PASSWORD", "PORTER_HTTP_ADDR", "PORTER_COOKIE_SECURE"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("PORTER_CLI_ENABLED", "true")
			t.Setenv("PORTER_ADMIN_USERNAME", "管理员")
			t.Setenv("PORTER_ADMIN_PASSWORD", "密码")
			t.Setenv("PORTER_HTTP_ADDR", "127.0.0.1:8080")
			t.Setenv("PORTER_COOKIE_SECURE", "false")
			value := "非法值"
			if key == "PORTER_ADMIN_USERNAME" || key == "PORTER_ADMIN_PASSWORD" {
				value = "  "
			}
			t.Setenv(key, value)
			t.Setenv("PORTER_WEB_ENABLED", "true")
			if _, err := Load(); err == nil {
				t.Fatal("Web启用时非法配置未拒绝")
			}
			t.Setenv("PORTER_WEB_ENABLED", "false")
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Username != "" || cfg.Password != "" || cfg.HTTPAddr != "" || cfg.CookieSecure {
				t.Fatalf("Web禁用后仍加载专用字段：%+v", cfg)
			}
			t.Setenv("PORTER_MAX_SESSIONS", "invalid")
			if _, err := Load(); err == nil {
				t.Fatal("关闭Web绕过共享转发配置校验")
			}
		})
	}
}
