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
