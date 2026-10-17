package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lwl0820/porter/internal/auth"
	"github.com/lwl0820/porter/internal/cli"
	"github.com/lwl0820/porter/internal/config"
	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/httpapi"
	"github.com/lwl0820/porter/internal/localapi"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/web"
)

var version = "dev"

func main() {
	first := ""
	if len(os.Args) > 1 {
		first, _, _ = strings.Cut(os.Args[1], "=")
	}
	if len(os.Args) > 1 && first != "--version" && first != "-version" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("Porter 运行失败", "error", err)
		os.Exit(1)
	}
}
func run() (result error) {
	showVersion := flag.Bool("version", false, "显示版本号")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	admin, err := db.EnsureAdmin(context.Background(), cfg.Username)
	if err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(cfg.HTTPAddr)
	network := "tcp4"
	if strings.Contains(host, ":") {
		network = "tcp6"
	}
	listener, err := net.Listen(network, cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("管理端口监听失败：%w", err)
	}
	defer listener.Close()
	service, err := rules.New(context.Background(), db, forward.New(forward.Limits{Global: cfg.MaxSessions, PerRule: cfg.MaxSessionsPerRule, UDPIdle: cfg.UDPIdle, DialTimeout: cfg.DialTimeout}), listener.Addr().String(), rules.Options{FlushInterval: cfg.TrafficFlushInterval})
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, service.Close())
		slog.Info("Porter 已停止")
	}()
	controlPath := os.Getenv("PORTER_CONTROL_SOCKET")
	if controlPath == "" {
		dbPath, pathErr := filepath.Abs(cfg.DBPath)
		if pathErr != nil {
			return pathErr
		}
		controlPath = filepath.Join(filepath.Dir(dbPath), "porter-control.sock")
	}
	control, err := localapi.Start(controlPath, service, admin)
	if err != nil {
		return fmt.Errorf("本机控制监听失败：%w", err)
	}
	defer func() { result = errors.Join(result, control.Close()) }()
	manager := auth.New(cfg.Username, cfg.Password)
	manager.SetUsers(db)
	handler := httpapi.New(service, manager, db, cfg.CookieSecure, web.Handler())
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	slog.Info("Porter 已启动", "address", listener.Addr().String(), "version", version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdown); err != nil {
		server.Close()
	}
	return err
}
