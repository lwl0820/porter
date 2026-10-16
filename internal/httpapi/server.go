package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lwl0820/porter/internal/auth"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/rules"
)

const cookieName = "porter_session"

type Pinger interface{ Ping(context.Context) error }
type Server struct {
	rules  *rules.Service
	auth   *auth.Manager
	db     Pinger
	secure bool
}

func New(service *rules.Service, manager *auth.Manager, db Pinger, secure bool, assets http.Handler) http.Handler {
	s := &Server{rules: service, auth: manager, db: db, secure: secure}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/auth/session", s.protect(s.session))
	mux.HandleFunc("POST /api/auth/logout", s.protect(s.logout))
	mux.HandleFunc("GET /api/rules", s.protect(s.list))
	mux.HandleFunc("POST /api/rules", s.protect(s.create))
	mux.HandleFunc("PUT /api/rules/{id}", s.protect(s.update))
	mux.HandleFunc("DELETE /api/rules/{id}", s.protect(s.remove))
	mux.HandleFunc("POST /api/rules/{id}/retry", s.protect(s.retry))
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		failure(w, &rules.Error{Status: 404, Code: "not_found", Message: "接口不存在"})
	})
	mux.Handle("/", assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) origin(r *http.Request) bool {
	value := r.Header.Get("Origin")
	if value == "" {
		return false
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	scheme := "http"
	if s.secure || r.TLS != nil {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host) && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/")
}

func (s *Server) protect(next func(http.ResponseWriter, *http.Request, auth.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			failure(w, &rules.Error{Status: 401, Code: "unauthorized", Message: "请先登录"})
			return
		}
		session, ok := s.auth.Get(cookie.Value)
		if !ok {
			failure(w, &rules.Error{Status: 401, Code: "unauthorized", Message: "登录已过期，请重新登录"})
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !s.origin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
				failure(w, &rules.Error{Status: 403, Code: "csrf_invalid", Message: "请求校验失败，请刷新页面后重试"})
				return
			}
		}
		next(w, r, session)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.origin(r) {
		failure(w, &rules.Error{Status: 403, Code: "origin_invalid", Message: "请求来源无效"})
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !s.auth.AllowLogin(ip) {
		w.Header().Set("Retry-After", "60")
		failure(w, &rules.Error{Status: 429, Code: "login_limited", Message: "登录尝试过于频繁，请稍后重试"})
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &input) {
		return
	}
	token, session, ok := s.auth.Login(input.Username, input.Password)
	if !ok {
		failure(w, &rules.Error{Status: 401, Code: "credentials_invalid", Message: "用户名或密码错误"})
		return
	}
	if old, err := r.Cookie(cookieName); err == nil {
		s.auth.Logout(old.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", Expires: session.Expires, MaxAge: 43200, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode})
	slog.Info("管理员已登录")
	write(w, 200, session)
}
func (s *Server) session(w http.ResponseWriter, r *http.Request, session auth.Session) {
	write(w, 200, session)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request, session auth.Session) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(204)
}
func (s *Server) list(w http.ResponseWriter, r *http.Request, session auth.Session) {
	write(w, 200, map[string]any{"rules": s.rules.List()})
}
func (s *Server) create(w http.ResponseWriter, r *http.Request, session auth.Session) {
	var input model.Input
	if !decode(w, r, &input) {
		return
	}
	rule, err := s.rules.Save(r.Context(), "", 0, input)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("ETag", etag(rule.Revision))
	write(w, 201, rule)
}
func (s *Server) update(w http.ResponseWriter, r *http.Request, session auth.Session) {
	revision, ok := match(w, r)
	if !ok {
		return
	}
	var input model.Input
	if !decode(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		failure(w, &rules.Error{Status: 400, Code: "validation_error", Message: "请提供规则启用状态", Fields: map[string]string{"enabled": "启用状态不能为空"}})
		return
	}
	rule, err := s.rules.Save(r.Context(), r.PathValue("id"), revision, input)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("ETag", etag(rule.Revision))
	write(w, 200, rule)
}
func (s *Server) remove(w http.ResponseWriter, r *http.Request, session auth.Session) {
	revision, ok := match(w, r)
	if !ok {
		return
	}
	if err := s.rules.Delete(r.Context(), r.PathValue("id"), revision); err != nil {
		failure(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) retry(w http.ResponseWriter, r *http.Request, session auth.Session) {
	revision, ok := match(w, r)
	if !ok {
		return
	}
	rule, err := s.rules.Retry(r.PathValue("id"), revision)
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, rule)
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		write(w, 503, map[string]string{"status": "unavailable"})
		return
	}
	write(w, 200, map[string]string{"status": "ok"})
}

func match(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v := r.Header.Get("If-Match")
	if len(v) < 3 || v[0] != '"' || v[len(v)-1] != '"' {
		failure(w, &rules.Error{Status: 400, Code: "revision_required", Message: "请提供当前规则版本"})
		return 0, false
	}
	n, err := strconv.ParseInt(v[1:len(v)-1], 10, 64)
	if err != nil || n < 1 {
		failure(w, &rules.Error{Status: 400, Code: "revision_invalid", Message: "规则版本无效"})
		return 0, false
	}
	return n, true
}
func etag(n int64) string { return `"` + strconv.FormatInt(n, 10) + `"` }
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	typeName, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typeName != "application/json" {
		failure(w, &rules.Error{Status: 400, Code: "json_required", Message: "请求必须使用 JSON"})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(target); err != nil {
		failure(w, &rules.Error{Status: 400, Code: "invalid_json", Message: "请求格式或字段无效"})
		return false
	}
	if err = d.Decode(&struct{}{}); err != io.EOF {
		failure(w, &rules.Error{Status: 400, Code: "invalid_json", Message: "请求只能包含一个 JSON 对象"})
		return false
	}
	return true
}
func failure(w http.ResponseWriter, err error) {
	var e *rules.Error
	if !errors.As(err, &e) {
		slog.Error("管理接口失败", "error", err)
		e = &rules.Error{Status: 500, Code: "internal_error", Message: "操作失败，请稍后重试"}
	}
	write(w, e.Status, map[string]any{"error": struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields,omitempty"`
	}{e.Code, e.Message, e.Fields}})
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Debug("响应写入失败", "error", err)
	}
}
