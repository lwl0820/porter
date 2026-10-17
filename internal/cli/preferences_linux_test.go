//go:build linux

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/testutil"
	"golang.org/x/sys/unix"
)

func TestExplicitLanguageMenuSwitchPersistsUsingCurrentInstallSnapshot(t *testing.T) {
	p, dir := preferenceFixture(t)
	writePreferenceTestFile(t, filepath.Join(dir, "install.json"), `{"schema":2,"status":"complete","version":"v1.0.0"}`)
	writePreferenceTestFile(t, filepath.Join(dir, "cli.json"), `{"schema":1,"language":"en"}`)
	r, output := scriptedRunner(t, "", "6\n2\n0\n", &fakeAPI{})
	var out, stderr bytes.Buffer
	rc := run(context.Background(), []string{"rules", "--lang=en"}, &out, &stderr, p, func(context.Context) (*terminal, error) {
		return r.tty, nil
	}, true, 0)
	if rc != 0 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("显式语言后的菜单切换失败：%d %s %s", rc, &out, &stderr)
	}
	if !strings.Contains(output(), "Rule management") || !strings.Contains(output(), "规则管理") {
		t.Fatal("语言切换未立即更新当前菜单")
	}
	next := &preferenceFiles{root: p.root}
	if language, err := next.Read(); err != nil || language != "zh-CN" {
		t.Fatal("下一次调用未读取切换后的语言", language, err)
	}
}

func preferenceFixture(t *testing.T) (*preferenceFiles, string) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("配置正向权限测试需隔离用户命名空间的真实UID0")
	}
	root := testutil.TempDir(t)
	dir := filepath.Join(root, "etc", "porter")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return &preferenceFiles{root: root}, dir
}
func writePreferenceTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestPreferenceAtomicRoundTripAndFirstWriterWins(t *testing.T) {
	p, dir := preferenceFixture(t)
	q := &preferenceFiles{root: p.root}
	for _, store := range []*preferenceFiles{p, q} {
		if lang, err := store.Read(); lang != "" || err != nil {
			t.Fatal(lang, err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for i, store := range []*preferenceFiles{p, q} {
		wg.Add(1)
		go func(i int, store *preferenceFiles) {
			defer wg.Done()
			lang := []string{"en", "zh-CN"}[i]
			got, err := store.Save(context.Background(), lang, false)
			if err != nil {
				t.Error(err)
			}
			results <- got
		}(i, store)
	}
	wg.Wait()
	close(results)
	var saved string
	for lang := range results {
		if saved != "" && saved != lang {
			t.Fatal("首次并发写覆盖已确认的设置")
		}
		saved = lang
	}
	check := &preferenceFiles{root: p.root}
	got, err := check.Read()
	if got != saved || err != nil {
		t.Fatal("持久化值不一致", got, saved, err)
	}
	if got, err = check.Save(context.Background(), "zh-CN", true); got != "zh-CN" || err != nil {
		t.Fatal(got, err)
	}
	st, err := os.Stat(filepath.Join(dir, "cli.json"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("权限不正确", st, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "cli.json" {
		t.Fatal("锁或临时文件残留", entries)
	}
}
func TestPreferenceUnsafeNodesNeverOverwritten(t *testing.T) {
	for _, kind := range []string{"malformed", "future", "large", "empty", "directory", "symlink", "dangling", "mode", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			p, dir := preferenceFixture(t)
			file := filepath.Join(dir, "cli.json")
			switch kind {
			case "directory":
				os.Mkdir(file, 0700)
			case "symlink":
				writePreferenceTestFile(t, filepath.Join(dir, "target"), `{"schema":1,"language":"en"}`)
				os.Symlink("target", file)
			case "dangling":
				os.Symlink("missing", file)
			case "mode":
				writePreferenceTestFile(t, file, `{"schema":1,"language":"en"}`)
				os.Chmod(file, 0644)
			case "hardlink":
				writePreferenceTestFile(t, file, `{"schema":1,"language":"en"}`)
				os.Link(file, filepath.Join(dir, "other"))
			case "future":
				writePreferenceTestFile(t, file, `{"schema":2,"language":"en"}`)
			case "large":
				writePreferenceTestFile(t, file, strings.Repeat(" ", 4097))
			case "empty":
				writePreferenceTestFile(t, file, "")
			default:
				writePreferenceTestFile(t, file, `{"schema":`)
			}
			before, _ := os.Lstat(file)
			if _, err := p.Read(); err == nil {
				t.Fatal("不可信配置读取成功")
			}
			if _, err := p.Save(context.Background(), "zh-CN", true); err == nil {
				t.Fatal("不可信配置被覆盖")
			}
			after, _ := os.Lstat(file)
			if !os.SameFile(before, after) {
				t.Fatal("拒绝后节点发生替换")
			}
		})
	}
}
func TestPreferenceUninstallRaceAndChangedStateRefuseSave(t *testing.T) {
	for _, kind := range []string{"uninstalling", "uninstalled", "upgrading", "removed-state", "replaced-state", "removed-directory", "replaced-directory"} {
		t.Run(kind, func(t *testing.T) {
			p, dir := preferenceFixture(t)
			state := filepath.Join(dir, "install.json")
			writePreferenceTestFile(t, state, `{"schema":2,"status":"complete","version":"v1.0.0"}`)
			if _, err := p.Read(); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "removed-directory":
				os.RemoveAll(dir)
			case "replaced-directory":
				os.Rename(dir, dir+"-old")
				os.Mkdir(dir, 0700)
			case "removed-state":
				os.Remove(state)
			case "replaced-state":
				writePreferenceTestFile(t, state, `{"schema":2,"status":"complete","version":"v2.0.0"}`)
			default:
				writePreferenceTestFile(t, state, `{"schema":2,"status":"`+kind+`"}`)
			}
			if _, err := p.Save(context.Background(), "en", false); err == nil {
				t.Fatal("安装变化仍写入配置")
			}
			if _, err := os.Lstat(filepath.Join(dir, "cli.json")); !os.IsNotExist(err) {
				t.Fatal("卸载期间创建了配置")
			}
			if kind == "removed-directory" {
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatal("重新创建了已卸载目录")
				}
			}
		})
	}
}
func TestPreferenceInstallLockAndCancellation(t *testing.T) {
	p, dir := preferenceFixture(t)
	if _, err := p.Read(); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, ".install-lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Save(ctx, "en", false); err == nil {
		t.Fatal("忽略安装锁")
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatal("清理了其他进程的锁")
	}
	if _, err := os.Stat(filepath.Join(dir, "cli.json")); !os.IsNotExist(err) {
		t.Fatal("中断后仍提交")
	}
	os.Remove(lock)
	if _, err := p.Save(context.Background(), "en", false); err != nil {
		t.Fatal(err)
	}
}

func TestPreferenceDirectorySafetyAndCreation(t *testing.T) {
	for _, kind := range []string{"missing", "unsafe", "directory-link", "parent-link"} {
		t.Run(kind, func(t *testing.T) {
			p, dir := preferenceFixture(t)
			switch kind {
			case "missing":
				os.Remove(dir)
			case "unsafe":
				os.Chmod(dir, 0777)
			case "directory-link":
				os.Rename(dir, dir+"-old")
				os.Symlink(dir+"-old", dir)
			case "parent-link":
				os.Rename(filepath.Dir(dir), filepath.Dir(dir)+"-old")
				os.Symlink(filepath.Dir(dir)+"-old", filepath.Dir(dir))
			}
			_, readErr := p.Read()
			if kind == "missing" {
				if readErr != nil {
					t.Fatal(readErr)
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, err := p.Save(ctx, "en", false); err == nil {
					t.Fatal("中断调用仍保存")
				}
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatal("中断后创建配置目录")
				}
				if _, err := p.Save(context.Background(), "en", false); err != nil {
					t.Fatal(err)
				}
			} else {
				if readErr == nil {
					t.Fatal("不可信目录被接受")
				}
				if _, err := p.Save(context.Background(), "en", false); err == nil {
					t.Fatal("向不可信目录保存")
				}
			}
		})
	}
}

func TestPreferenceUnprivilegedOwnerRejected(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("常规非root进程验证不可信属主")
	}
	root := testutil.TempDir(t)
	dir := filepath.Join(root, "etc", "porter")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	writePreferenceTestFile(t, filepath.Join(dir, "cli.json"), `{"schema":1,"language":"en"}`)
	p := &preferenceFiles{root: root}
	if _, err := p.Read(); err == nil {
		t.Fatal("非root属主的配置目录被接受")
	}
	if _, err := p.Save(context.Background(), "zh-CN", true); err == nil {
		t.Fatal("非root调用保存了系统偏好")
	}
}

func TestPreferenceModeRemains0600WithRestrictiveUmask(t *testing.T) {
	p, dir := preferenceFixture(t)
	if _, err := p.Read(); err != nil {
		t.Fatal(err)
	}
	previous := unix.Umask(0777)
	_, err := p.Save(context.Background(), "en", false)
	unix.Umask(previous)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "cli.json"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("配置权限受umask破坏", st, err)
	}
}
