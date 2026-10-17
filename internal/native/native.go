// Package native 界定仅 Linux 原生环境可用的本机管理功能。
package native

import (
	"os"
	"runtime"
	"strings"
)

func Available() bool {
	if dockerBuild || runtime.GOOS != "linux" {
		return false
	}
	for _, path := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(path); err == nil {
			return false
		}
	}
	if os.Getenv("container") != "" {
		return false
	}
	for _, path := range []string{"/proc/1/cgroup", "/proc/self/cgroup"} {
		data, _ := os.ReadFile(path)
		for _, marker := range []string{"docker", "kubepods", "containerd", "libpod", "lxc"} {
			if strings.Contains(string(data), marker) {
				return false
			}
		}
	}
	return true
}
