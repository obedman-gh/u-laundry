package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"ulaundry/internal/http/middleware"
	"ulaundry/internal/testutil"
)

func TestSecurityHeadersAndPublicAssetBoundary(t *testing.T) {
	assets := http.FS(fstest.MapFS{
		"index.html":       {Data: []byte("public-shell")},
		"login.html":       {Data: []byte("public-login")},
		".env":             {Data: []byte("PRIVATE_SECRET")},
		"docs/security.md": {Data: []byte("PRIVATE_DOC")},
		"ui/leak.js":       {Data: []byte("PRIVATE_BACKUP")},
		"login.js.bak":     {Data: []byte("PRIVATE_BACKUP")},
	})
	manager := testutil.TestSessions("synthetic-router-test-secret-32bytes")
	manager.Secure = true
	router := NewRouter(&endpointRecorder{}, middleware.New(nil, manager), assets)
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/", 200}, {"/login.html", 200}, {"/index.html", 301},
		{"/.env", 404}, {"/%2eenv", 404}, {"/docs/security.md", 404},
		{"/ui/", 404}, {"/ui/leak.js", 404}, {"/login.js.bak", 404},
		{"/api/pesanan/order-1/struk", 401}, {"/api/unknown", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.status || strings.Contains(w.Body.String(), "PRIVATE_") {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			for key, want := range map[string]string{
				"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
				"Referrer-Policy": "no-referrer", "Cache-Control": "no-store",
				"Cross-Origin-Opener-Policy": "same-origin", "Cross-Origin-Resource-Policy": "same-origin",
				"Strict-Transport-Security": "max-age=31536000", "X-Robots-Tag": "noindex, nofollow",
			} {
				if w.Header().Get(key) != want {
					t.Errorf("%s=%q", key, w.Header().Get(key))
				}
			}
			csp := w.Header().Get("Content-Security-Policy")
			for _, directive := range []string{"script-src 'self'", "style-src 'self'", "frame-ancestors 'none'", "base-uri 'none'", "worker-src 'self'", "form-action 'self'"} {
				if !strings.Contains(csp, directive) {
					t.Error("missing CSP", directive)
				}
			}
			if strings.Contains(csp, "unsafe-") || strings.Contains(csp, "*") {
				t.Fatal("permissive CSP")
			}
		})
	}
	// Even a missing index must not reveal a directory listing.
	rangeRequest := httptest.NewRequest("GET", "/login.html", nil)
	rangeRequest.Header.Set("Range", "bytes=999999-")
	rangeResponse := httptest.NewRecorder()
	router.ServeHTTP(rangeResponse, rangeRequest)
	if rangeResponse.Code != 416 || rangeResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("range error lost privacy policy")
	}
	router = NewRouter(&endpointRecorder{}, middleware.New(nil, manager), http.FS(fstest.MapFS{"login.html": {Data: []byte("PRIVATE_LISTING")}}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 404 || strings.Contains(w.Body.String(), "PRIVATE_LISTING") {
		t.Fatal("directory listed")
	}
	manager.Secure = false
	w = httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/login.html", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	router.ServeHTTP(w, r)
	if w.Code != 405 || w.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("static method or spoofed HSTS")
	}
}

func TestAPIBodyLimitsBeforeEndpoints(t *testing.T) {
	for _, path := range []string{"/api/login", "/api/logout", "/api/unknown", "/api/tracking/token", "/api/pesanan/order-1/struk"} {
		for _, chunked := range []bool{false, true} {
			e := &endpointRecorder{}
			router := NewRouter(e, middleware.New(nil, nil), http.FS(fstest.MapFS{}))
			r := httptest.NewRequest("POST", path, strings.NewReader(strings.Repeat("x", int(middleware.MaxBodyBytes)+1)))
			if chunked {
				r.ContentLength = -1
				r.TransferEncoding = []string{"chunked"}
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != 413 || e.name != "" || !strings.Contains(w.Body.String(), `"code":"payload_too_large"`) {
				t.Fatalf("%s chunked=%v status=%d", path, chunked, w.Code)
			}
		}
	}
}
