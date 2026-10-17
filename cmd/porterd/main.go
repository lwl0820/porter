package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"github.com/lwl0820/porter/internal/config"
	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/httpapi"
	"github.com/lwl0820/porter/internal/localapi"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/native"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/web"
)

var version = "dev"

type arguments struct {
	showVersion bool
	checkConfig bool
}

func parseArguments(args []string, stderr io.Writer) (arguments, error) {
	flags := flag.NewFlagSet("porterd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "用法：porterd [--version | --check-config]")
		flags.PrintDefaults()
	}
	showVersion := flags.Bool("version", false, "显示版本号")
	checkConfig := flags.Bool("check-config", false, "校验配置与当前环境管理能力，不启动服务")
	if err := flags.Parse(args); err != nil {
		return arguments{}, err
	}
	if flags.NArg() != 0 {
		return arguments{}, fmt.Errorf("porterd 不接受客户端命令，请使用 porter：%s", flags.Arg(0))
	}
	if *showVersion && *checkConfig {
		return arguments{}, fmt.Errorf("--version 与 --check-config 不能同时使用")
	}
	return arguments{showVersion: *showVersion, checkConfig: *checkConfig}, nil
}

func main() {
	args, err := parseArguments(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if args.showVersion {
		fmt.Println(version)
		return
	}
	if args.checkConfig {
		if _, err = loadConfig(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("Porter 运行失败", "error", err)
		os.Exit(1)
	}
}
func validateManagement(cfg config.Config, nativeAvailable bool) error {
	if !cfg.WebEnabled && !(cfg.CLIEnabled && nativeAvailable) {
		return fmt.Errorf("当前环境不支持本机命令行管理，必须启用 Web 服务")
	}
	return nil
}

func loadConfig() (config.Config, error) {
	cfg, err := config.Load()
	if err == nil {
		err = validateManagement(cfg, native.Available())
	}
	return cfg, err
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWithContext(ctx, cfg)
}

func runWithContext(ctx context.Context, cfg config.Config) (result error) {
	if err := validateManagement(cfg, native.Available()); err != nil {
		return err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	var admin model.User
	if cfg.WebEnabled {
		admin, err = db.EnsureAdmin(ctx, cfg.Username)
	} else {
		admin, err = db.GetUser(ctx, model.AdminID)
	}
	if err != nil {
		return err
	}
	var listener net.Listener
	adminAddr := ""
	if cfg.WebEnabled {
		host, _, _ := net.SplitHostPort(cfg.HTTPAddr)
		network := "tcp4"
		if strings.Contains(host, ":") {
			network = "tcp6"
		}
		listener, err = net.Listen(network, cfg.HTTPAddr)
		if err != nil {
			return fmt.Errorf("管理端口监听失败：%w", err)
		}
		defer listener.Close()
		adminAddr = listener.Addr().String()
	}
	service, err := rules.New(ctx, db, forward.New(forward.Limits{Global: cfg.MaxSessions, PerRule: cfg.MaxSessionsPerRule, UDPIdle: cfg.UDPIdle, DialTimeout: cfg.DialTimeout}), adminAddr, rules.Options{FlushInterval: cfg.TrafficFlushInterval})
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, service.Close())
		slog.Info("Porter 已停止")
	}()
	if cfg.CLIEnabled && native.Available() {
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
	}
	var done <-chan error
	var server *http.Server
	if cfg.WebEnabled {
		manager := auth.New(cfg.Username, cfg.Password)
		manager.SetUsers(db)
		handler := httpapi.New(service, manager, db, cfg.CookieSecure, web.Handler())
		server = &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		serveErrors := make(chan error, 1)
		done = serveErrors
		go func() { serveErrors <- server.Serve(listener) }()
		defer server.Close()
	}
	slog.Info("Porter 已启动", "address", adminAddr, "web_enabled", cfg.WebEnabled, "cli_enabled", cfg.CLIEnabled && native.Available(), "version", version)
	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	if server == nil {
		return nil
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
