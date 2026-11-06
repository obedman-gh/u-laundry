package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"
	"time"

	"ulaundry/internal/http/middleware"

	"github.com/golang-jwt/jwt/v5"
)

type endpointRecorder struct {
	name string
	id   string
}

func (e *endpointRecorder) record(name string, w http.ResponseWriter, r *http.Request) {
	e.name, e.id = name, r.PathValue("id")
	w.WriteHeader(http.StatusNoContent)
}

func (e *endpointRecorder) HealthCheck(w http.ResponseWriter, r *http.Request) {
	e.record("health", w, r)
}
func (e *endpointRecorder) GetPesananById(w http.ResponseWriter, r *http.Request) {
	e.record("detail", w, r)
}
func (e *endpointRecorder) CetakStruk(w http.ResponseWriter, r *http.Request) {
	e.record("receipt", w, r)
}
func (e *endpointRecorder) Login(w http.ResponseWriter, r *http.Request) {
	e.record("login", w, r)
}
func (e *endpointRecorder) GetSemuaPesanan(w http.ResponseWriter, r *http.Request) {
	e.record("list", w, r)
}
func (e *endpointRecorder) GetPesananByStatus(w http.ResponseWriter, r *http.Request) {
	e.record("filter", w, r)
}
func (e *endpointRecorder) BikinPesananBaru(w http.ResponseWriter, r *http.Request) {
	e.record("create", w, r)
}
func (e *endpointRecorder) UpdateStatusPesanan(w http.ResponseWriter, r *http.Request) {
	e.record("update", w, r)
}
func (e *endpointRecorder) GetNeraca(w http.ResponseWriter, r *http.Request) {
	e.record("report", w, r)
}

func TestExistingRoutes(t *testing.T) {
	const secret = "synthetic-router-test-secret"
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "fixture", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, url, endpoint, id string
		private                   bool
	}{
		{"GET", "/api/health", "health", "", false},
		{"GET", "/api/pesanan/ORD-123", "detail", "ORD-123", false},
		{"GET", "/api/pesanan/ORD-123/struk", "receipt", "ORD-123", false},
		{"POST", "/api/login", "login", "", false},
		{"GET", "/api/pesanan", "list", "", true},
		{"GET", "/api/pesanan/status?q=Selesai", "filter", "", true},
		{"POST", "/api/pesanan", "create", "", true},
		{"PUT", "/api/pesanan/ORD-123", "update", "ORD-123", true},
		{"GET", "/api/laporan/neraca", "report", "", true},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			e := &endpointRecorder{}
			router := NewRouter(e, middleware.New(secret, "https://fixture.invalid"), http.FS(fstest.MapFS{}))
			req := httptest.NewRequest(tc.method, tc.url, nil)
			if tc.private {
				denied := httptest.NewRecorder()
				router.ServeHTTP(denied, req)
				if denied.Code != http.StatusUnauthorized || e.name != "" {
					t.Fatalf("private route reached endpoint without auth: status=%d endpoint=%q", denied.Code, e.name)
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusNoContent || e.name != tc.endpoint || e.id != tc.id {
				t.Fatalf("status=%d endpoint=%q id=%q; want %d %q %q", rr.Code, e.name, e.id, http.StatusNoContent, tc.endpoint, tc.id)
			}
			if origin := rr.Header().Get("Access-Control-Allow-Origin"); origin != "https://fixture.invalid" {
				t.Fatalf("CORS wrapper missing: %q", origin)
			}
		})
	}
}

func TestExistingStaticAssets(t *testing.T) {
	// Locate source assets without changing or depending on the working directory.
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate frontend fixtures")
	}
	assets := filepath.Join(filepath.Dir(sourceFile), "..", "..", "frontend")
	router := NewRouter(&endpointRecorder{}, middleware.New("fixture", ""), http.Dir(assets))
	for _, tc := range []struct{ url, file string }{{"/", "index.html"}, {"/login.html", "login.html"}} {
		t.Run(tc.file, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(assets, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, httptest.NewRequest("GET", tc.url, nil))
			if rr.Code != http.StatusOK || !bytes.Equal(rr.Body.Bytes(), want) {
				t.Fatalf("static asset changed or unavailable: status=%d", rr.Code)
			}
			if rr.Header().Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatal("static asset content type changed")
			}
		})
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest("GET", "/index.html", nil))
	if rr.Code != http.StatusMovedPermanently || rr.Header().Get("Location") != "./" {
		t.Fatal("legacy index redirect changed")
	}
}

func TestExistingMethodHandling(t *testing.T) {
	for _, method := range []string{"DELETE", "OPTIONS"} {
		t.Run(method, func(t *testing.T) {
			e := &endpointRecorder{}
			router := NewRouter(e, middleware.New("fixture", ""), http.FS(fstest.MapFS{}))
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, httptest.NewRequest(method, "/api/pesanan/ORD-123", nil))
			// The catch-all static route handles unmatched API methods;
			// these missing file paths preserve the legacy 404 response.
			if rr.Code != http.StatusNotFound || e.name != "" {
				t.Fatalf("legacy method handling changed: status=%d endpoint=%q", rr.Code, e.name)
			}
		})
	}
}
