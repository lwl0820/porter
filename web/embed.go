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
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		server.ServeHTTP(w, r)
	})
}
