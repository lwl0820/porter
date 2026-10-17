package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Username             string
	Password             string
	HTTPAddr             string
	DBPath               string
	CookieSecure         bool
	MaxSessions          int
	MaxSessionsPerRule   int
	UDPIdle              time.Duration
	DialTimeout          time.Duration
	TrafficFlushInterval time.Duration
}

func Load() (Config, error) {
	c := Config{Username: strings.TrimSpace(os.Getenv("PORTER_ADMIN_USERNAME")), Password: os.Getenv("PORTER_ADMIN_PASSWORD"), HTTPAddr: value("PORTER_HTTP_ADDR", "0.0.0.0:8080"), DBPath: value("PORTER_DB_PATH", "/data/porter.db")}
	if c.Username == "" || strings.TrimSpace(c.Password) == "" {
		return c, fmt.Errorf("必须设置 PORTER_ADMIN_USERNAME 和 PORTER_ADMIN_PASSWORD")
	}
	host, port, addrErr := net.SplitHostPort(c.HTTPAddr)
	if addrErr != nil || net.ParseIP(host) == nil {
		return c, fmt.Errorf("PORTER_HTTP_ADDR 必须是有效的 IP:端口")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("PORTER_HTTP_ADDR 端口范围为 1–65535")
	}
	var err error
	if c.CookieSecure, err = strconv.ParseBool(value("PORTER_COOKIE_SECURE", "false")); err != nil {
		return c, fmt.Errorf("PORTER_COOKIE_SECURE 必须是布尔值")
	}
	if c.MaxSessions, err = positive("PORTER_MAX_SESSIONS", 4096); err != nil {
		return c, err
	}
	if c.MaxSessionsPerRule, err = positive("PORTER_MAX_SESSIONS_PER_RULE", 1024); err != nil {
		return c, err
	}
	if c.UDPIdle, err = duration("PORTER_UDP_IDLE_TIMEOUT", "60s"); err != nil {
		return c, err
	}
	if c.DialTimeout, err = duration("PORTER_DIAL_TIMEOUT", "5s"); err != nil {
		return c, err
	}
	if c.TrafficFlushInterval, err = duration("PORTER_TRAFFIC_FLUSH_INTERVAL", "5s"); err != nil {
		return c, err
	}
	return c, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func positive(key string, fallback int) (int, error) {
	n, e := strconv.Atoi(value(key, strconv.Itoa(fallback)))
	if e != nil || n < 1 || n > 1000000 {
		return 0, fmt.Errorf("%s 必须是 1–1000000 的整数", key)
	}
	return n, nil
}
func duration(key, fallback string) (time.Duration, error) {
	d, e := time.ParseDuration(value(key, fallback))
	if e != nil || d < time.Millisecond || d > 24*time.Hour {
		return 0, fmt.Errorf("%s 必须是 1ms–24h 的有效时长", key)
	}
	return d, nil
}
