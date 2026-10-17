// Package web 提供随程序发布的前端静态资源。
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler 返回静态资源处理器，管理接口由上层路由独立分派。
func Handler() http.Handler {
	files, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/admin/", http.StatusFound)
			return
		}
		if r.URL.Path == "/admin" || r.URL.Path == "/user" {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		// 仅明确的前端页面可映射首页，接口和不存在的静态资源保持 404。
		page := ""
		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/admin", "/admin/login", "/admin/rules", "/admin/plans", "/admin/users", "/admin/settings":
			page = "/admin/"
		case "/user", "/user/login", "/user/rules", "/user/account":
			page = "/user/"
		}
		if page != "" {
			r = r.Clone(r.Context())
			r.URL.Path = page
		} else if !strings.HasPrefix(r.URL.Path, "/assets/") && r.URL.Path != "/favicon.svg" {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		server.ServeHTTP(w, r)
	})
}
