// Package testutil 将测试临时目录限定在仓库的 .tmp 下。
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TempDir(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("找不到仓库根目录")
		}
		root = parent
	}
	base := filepath.Join(root, ".tmp", "tests")
	if err = os.MkdirAll(base, 0750); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "porter-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	return dir
}
