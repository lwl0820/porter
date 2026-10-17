package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"github.com/lwl0820/porter/internal/config"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerArgumentsRejectClientCommands(t *testing.T) {
	for _, args := range [][]string{{"rules"}, {"rules", "--lang", "en"}, {"--unknown"}, {"--version", "rules"}} {
		if _, err := parseArguments(args, io.Discard); err == nil {
			t.Fatalf("服务接收客户端参数：%v", args)
		}
	}
	for _, arg := range []string{"--version", "-version"} {
		if version, err := parseArguments([]string{arg}, io.Discard); !version.showVersion || err != nil {
			t.Fatal(version, err)
		}
	}
}

func TestConfigCheckArguments(t *testing.T) {
	args, err := parseArguments([]string{"--check-config"}, io.Discard)
	if err != nil || !args.checkConfig || args.showVersion {
		t.Fatal(args, err)
	}
	for _, args := range [][]string{{"--check-config", "rules"}, {"--check-config", "--version"}} {
		if _, err := parseArguments(args, io.Discard); err == nil {
			t.Fatal("冲突参数未拒绝", args)
		}
	}
	var help bytes.Buffer
	if _, err := parseArguments([]string{"--help"}, &help); !errors.Is(err, flag.ErrHelp) || !strings.Contains(help.String(), "--check-config") {
		t.Fatal("帮助缺少能力发现标识", help.String(), err)
	}
}

func TestManagementAvailability(t *testing.T) {
	for _, available := range []bool{false, true} {
		for _, test := range []struct{ web, cli bool }{{true, true}, {true, false}, {false, true}, {false, false}} {
			err := validateManagement(config.Config{WebEnabled: test.web, CLIEnabled: test.cli}, available)
			want := test.web || test.cli && available
			if (err == nil) != want {
				t.Fatalf("组合%+v，原生%v：%v", test, available, err)
			}
		}
	}
}

func TestConfigCheckHasNoSideEffects(t *testing.T) {
	if os.Getenv("PORTER_CONFIG_CHECK_HELPER") == "1" {
		os.Args = []string{"porterd", "--check-config"}
		main()
		return
	}
	base := testutil.TempDir(t)
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	for _, bad := range []bool{false, true} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestConfigCheckHasNoSideEffects$")
		cmd.Env = append(os.Environ(), "PORTER_CONFIG_CHECK_HELPER=1", "PORTER_WEB_ENABLED=true", "PORTER_CLI_ENABLED=false", "PORTER_ADMIN_USERNAME=预检管理员", "PORTER_ADMIN_PASSWORD=不应写入文件的密码", "PORTER_HTTP_ADDR="+occupied.Addr().String(), "PORTER_DB_PATH="+filepath.Join(base, "missing", "porter.db"), "PORTER_CONTROL_SOCKET="+filepath.Join(base, "control.sock"))
		if bad {
			cmd.Env = append(cmd.Env, "PORTER_HTTP_ADDR=invalid")
		}
		output, err := cmd.CombinedOutput()
		if (err != nil) != bad {
			t.Fatalf("预检结果：%s %v", output, err)
		}
		if strings.Contains(string(output), "不应写入文件的密码") {
			t.Fatal("预检泄漏密码")
		}
		files, err := os.ReadDir(base)
		if err != nil || len(files) != 0 {
			t.Fatal("预检创建了文件", files, err)
		}
	}
}

func TestWebOnlyLifecycle(t *testing.T) {
	base := testutil.TempDir(t)
	dbPath := filepath.Join(base, "porter.db")
	controlPath := filepath.Join(base, "control.sock")
	t.Setenv("PORTER_CONTROL_SOCKET", controlPath)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runWithContext(ctx, config.Config{WebEnabled: true, CLIEnabled: false, Username: "管理员", Password: "测试密码", HTTPAddr: address, DBPath: dbPath})
	}()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get("http://" + address + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("Web未就绪", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(controlPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("仅Web模式创建了本机入口", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Web退出超时")
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal("退出未释放数据库", err)
	}
	defer db.Close()
	admin, err := db.GetUser(context.Background(), model.AdminID)
	if err != nil || admin.Username != "管理员" {
		t.Fatal(admin, err)
	}
}
