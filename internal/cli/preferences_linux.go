//go:build linux

package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// 所有路径通过可信目录描述符访问；root仅用于隔离测试的根目录注入。
type preferenceFiles struct {
	root      string
	read      bool
	directory *unix.Stat_t
	state     []byte
}

func newPreferences() preferenceStore { return &preferenceFiles{root: "/"} }

func trustedPreferenceDirectory(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != 0 || st.Mode&0022 != 0 {
		return cliError("preference_invalid")
	}
	return nil
}

func (p *preferenceFiles) openDirectory(create bool) (int, error) {
	fd, err := unix.Open(p.root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range []string{"etc", "porter"} {
		if err = trustedPreferenceDirectory(fd); err != nil {
			unix.Close(fd)
			return -1, err
		}
		next, e := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(e, unix.ENOENT) && create && component == "porter" {
			e = unix.Mkdirat(fd, component, 0700)
			if e == nil || errors.Is(e, unix.EEXIST) {
				next, e = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			}
		}
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	if err = trustedPreferenceDirectory(fd); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func readPreferenceFile(fd int, name string, limit int64) ([]byte, error) {
	n, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(n), name)
	defer file.Close()
	var st unix.Stat_t
	if err = unix.Fstat(n, &st); err != nil {
		return nil, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != 0 || st.Mode&07777 != 0600 || st.Nlink != 1 || st.Size > limit {
		return nil, cliError("preference_invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, cliError("preference_invalid")
	}
	// 空文件与缺失不同，前者不能被当作首次配置覆盖。
	if data == nil {
		data = []byte{}
	}
	return data, nil
}

func (p *preferenceFiles) Read() (string, error) {
	fd, err := p.openDirectory(false)
	p.read = true
	if errors.Is(err, unix.ENOENT) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return "", err
	}
	p.directory = &st
	p.state, err = readPreferenceFile(fd, "install.json", 65536)
	if err != nil {
		return "", err
	}
	data, err := readPreferenceFile(fd, "cli.json", 4096)
	if err != nil || data == nil {
		return "", err
	}
	return decodePreference(data)
}

func stateAllowsPreferences(data []byte) bool {
	if data == nil {
		return true
	}
	var state struct {
		Schema int    `json:"schema"`
		Status string `json:"status"`
	}
	if json.Unmarshal(data, &state) != nil || (state.Schema != 1 && state.Schema != 2) {
		return false
	}
	return state.Status == "complete" || state.Status == "initializing"
}

func (p *preferenceFiles) Save(ctx context.Context, language string, overwrite bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if language != "en" && language != "zh-CN" {
		return "", cliError("language_invalid")
	}
	if os.Geteuid() != 0 {
		return "", cliError("preference_readonly")
	}
	if !p.read {
		if _, err := p.Read(); err != nil {
			return "", err
		}
	}
	// 之前存在的目录消失时不能重建，尤其是等待输入期间发生了purge。
	fd, err := p.openDirectory(p.directory == nil)
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	var current unix.Stat_t
	if err = unix.Fstat(fd, &current); err != nil {
		return "", err
	}
	if p.directory != nil && (p.directory.Dev != current.Dev || p.directory.Ino != current.Ino) {
		return "", cliError("preference_save_failed")
	}
	// 与安装器共享mkdir锁；不持锁等待语言输入，也不清理其他进程的锁。
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		err = unix.Mkdirat(fd, ".install-lock", 0700)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EEXIST) {
			return "", err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-deadline.C:
			timer.Stop()
			return "", cliError("preference_save_failed")
		case <-timer.C:
		}
	}
	var lockStat unix.Stat_t
	if err = unix.Fstatat(fd, ".install-lock", &lockStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return "", err
	}
	defer func() {
		var latest unix.Stat_t
		if unix.Fstatat(fd, ".install-lock", &latest, unix.AT_SYMLINK_NOFOLLOW) == nil && latest.Dev == lockStat.Dev && latest.Ino == lockStat.Ino {
			_ = unix.Unlinkat(fd, ".install-lock", unix.AT_REMOVEDIR)
		}
	}()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	// 父目录可能在获取锁之前被安装器移除或替换；只有仍位于同一路径才写。
	actual, err := p.openDirectory(false)
	if err != nil {
		return "", err
	}
	var actualStat unix.Stat_t
	err = unix.Fstat(actual, &actualStat)
	unix.Close(actual)
	if err != nil {
		return "", err
	}
	if actualStat.Dev != current.Dev || actualStat.Ino != current.Ino {
		return "", cliError("preference_save_failed")
	}
	state, err := readPreferenceFile(fd, "install.json", 65536)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(state, p.state) || (state == nil) != (p.state == nil) || !stateAllowsPreferences(state) {
		return "", cliError("preference_save_failed")
	}
	data, err := readPreferenceFile(fd, "cli.json", 4096)
	if err != nil {
		return "", err
	}
	if data != nil {
		existing, e := decodePreference(data)
		if e != nil {
			return "", e
		}
		if !overwrite {
			return existing, nil
		}
	}
	var suffix [12]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		return "", err
	}
	temporary := ".cli-" + hex.EncodeToString(suffix[:]) + ".new"
	n, err := unix.Openat(fd, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return "", err
	}
	defer unix.Unlinkat(fd, temporary, 0)
	file := os.NewFile(uintptr(n), temporary)
	encoded, _ := json.Marshal(languagePreference{Schema: 1, Language: language})
	err = file.Chmod(0600)
	if err == nil {
		_, err = file.Write(append(encoded, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = unix.Renameat(fd, temporary, fd, "cli.json"); err != nil {
		return "", err
	}
	if err = unix.Fsync(fd); err != nil {
		return "", err
	}
	p.directory = &current
	p.state = append([]byte(nil), state...)
	return language, nil
}

// 不向用户回显来自不可信配置的文本或路径内容。
func preferenceReadCode(err error) string {
	if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
		return "preference_unreadable"
	}
	return "preference_invalid"
}
