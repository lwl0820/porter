//go:build linux && !porter_docker

package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/native"
	"github.com/lwl0820/porter/internal/rules"
	"golang.org/x/sys/unix"
)

type peerKey struct{}
type Server struct {
	server    *http.Server
	listener  *net.UnixListener
	path      string
	node      os.FileInfo
	done      chan error
	closeOnce sync.Once
	closeErr  error
}

// Start 在不支持本机管理的环境中不创建任何入口。
func Start(path string, service *rules.Service, admin model.User) (*Server, error) {
	if !native.Available() {
		return nil, nil
	}
	return start(path, service, admin)
}
func start(path string, service *rules.Service, admin model.User) (*Server, error) {
	if admin.ID != model.AdminID || admin.Role != model.RoleAdmin {
		return nil, fmt.Errorf("本机控制缺少权威管理员")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("控制套接字必须使用绝对路径")
	}
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	for dir := parent; ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return nil, err
		}
		if !trustedDirectory(info) {
			return nil, fmt.Errorf("控制套接字目录不可由其他用户写入或使用符号链接：%s", dir)
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	dir, err := os.Open(parent)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	// 用目录锁串行化探测、清理和绑定，不留下新的锁文件。
	if err = unix.Flock(int(dir.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, fmt.Errorf("控制目录正在初始化：%w", err)
	}
	defer unix.Flock(int(dir.Fd()), unix.LOCK_UN)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("控制路径已存在且不是套接字")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) {
			return nil, fmt.Errorf("拒绝清理不属于服务用户的套接字")
		}
		conn, dialErr := net.DialTimeout("unix", path, time.Second)
		if dialErr == nil {
			conn.Close()
			return nil, fmt.Errorf("控制套接字正在使用")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("无法确认控制套接字已失效：%w", dialErr)
		}
		latest, err := os.Lstat(path)
		if err != nil || !os.SameFile(info, latest) {
			return nil, fmt.Errorf("控制套接字在检查期间被替换")
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	info, err := os.Lstat(path)
	if err != nil {
		listener.Close()
		return nil, err
	}
	s := &Server{listener: listener, path: path, node: info, done: make(chan error, 1)}
	if err = os.Chmod(path, 0600); err != nil {
		listener.Close()
		s.cleanup()
		return nil, err
	}
	handler := controlHandler(service, admin)
	s.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, _ := r.Context().Value(peerKey{}).(bool)
		if !allowed {
			failure(w, &Error{Status: 403, Code: "forbidden"})
			return
		}
		handler.ServeHTTP(w, r)
	}), ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, peerKey{}, rootPeer(c))
	}, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	go func() { s.done <- s.server.Serve(listener) }()
	return s, nil
}

func trustedDirectory(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	// 目录所有者即使当前没有写权限，也能自行 chmod 后替换下层路径。
	return info.IsDir() && info.Mode()&0022 == 0 && ok && (stat.Uid == 0 || stat.Uid == uint32(os.Geteuid()))
}

func rootPeer(c net.Conn) bool {
	conn, ok := c.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	allowed := false
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		allowed = e == nil && cred.Uid == 0
	})
	return err == nil && allowed
}
func (s *Server) cleanup() error {
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if os.SameFile(s.node, info) {
		return os.Remove(s.path)
	}
	return nil
}
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		err := s.server.Shutdown(ctx)
		if err != nil {
			s.server.Close()
		}
		serveErr := <-s.done
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		s.closeErr = errors.Join(err, serveErr, s.cleanup())
	})
	return s.closeErr
}
func controlHandler(service *rules.Service, admin model.User) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+resource, func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, RuleList{Rules: service.ListFor(admin, r.URL.Query().Get("owner_id"))})
	})
	mux.HandleFunc("GET "+resource+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := service.GetFor(admin, r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		ruleResponse(w, 200, value)
	})
	mux.HandleFunc("POST "+resource, func(w http.ResponseWriter, r *http.Request) {
		var in model.AdminRuleCreateInput
		if !decode(w, r, &in) {
			return
		}
		value, err := service.SaveFor(r.Context(), admin, in.OwnerID, "", 0, in.Input)
		if err != nil {
			failure(w, err)
			return
		}
		ruleResponse(w, 201, value)
	})
	mux.HandleFunc("PUT "+resource+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		rev, err := revision(r)
		if err != nil {
			failure(w, err)
			return
		}
		var in model.Input
		if !decode(w, r, &in) {
			return
		}
		value, err := service.SaveFor(r.Context(), admin, "", r.PathValue("id"), rev, in)
		if err != nil {
			failure(w, err)
			return
		}
		ruleResponse(w, 200, value)
	})
	mux.HandleFunc("DELETE "+resource+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		rev, err := revision(r)
		if err != nil {
			failure(w, err)
			return
		}
		if err = service.DeleteFor(r.Context(), admin, r.PathValue("id"), rev); err != nil {
			failure(w, err)
			return
		}
		w.WriteHeader(204)
	})
	return mux
}
func ruleResponse(w http.ResponseWriter, status int, value model.Rule) {
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, value.Revision))
	write(w, status, value)
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, err error) {
	var e *Error
	var business *rules.Error
	if errors.As(err, &business) {
		e = &Error{Status: business.Status, Code: business.Code, Message: business.Message, Fields: business.Fields}
	} else if !errors.As(err, &e) {
		e = &Error{Status: 500, Code: "internal_error"}
	}
	write(w, e.Status, map[string]any{"error": e})
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, &Error{Status: 400, Code: "json_required"})
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	if err = d.Decode(target); err != nil {
		failure(w, &Error{Status: 400, Code: "invalid_json"})
		return false
	}
	if err = d.Decode(&struct{}{}); err != io.EOF {
		failure(w, &Error{Status: 400, Code: "invalid_json"})
		return false
	}
	return true
}
