package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndependentEntrypoints(t *testing.T) {
	handler := Handler()
	for _, tc := range []struct{ path, marker string }{
		{"/admin/", "admin-"}, {"/admin/login", "admin-"}, {"/admin/rules", "admin-"}, {"/admin/settings", "admin-"},
		{"/user/", "user-"}, {"/user/login", "user-"}, {"/user/rules", "user-"}, {"/user/account", "user-"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.marker) || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("页面映射错误：状态=%d，内容=%s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-cache" {
				t.Fatal("页面不能长期缓存")
			}
		})
	}
	for _, path := range []string{"/api/rules", "/api/admin/missing", "/api/user/missing", "/admin/missing", "/assets/missing.js"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "<!doctype") {
			t.Fatalf("%s 不应兜底成首页", path)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/admin/" {
		t.Fatal("根地址应跳转管理端")
	}
}
