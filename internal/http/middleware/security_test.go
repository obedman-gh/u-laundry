package middleware

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failedBody struct{}

func (failedBody) Read([]byte) (int, error) { return 0, errors.New("private-driver-detail") }
func (failedBody) Close() error             { return nil }

func TestBodyLimitBoundaryAndReadFailure(t *testing.T) {
	for _, size := range []int{0, int(MaxBodyBytes), int(MaxBodyBytes) + 1} {
		called := false
		h := RequestLimits(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			body, err := io.ReadAll(r.Body)
			if err != nil || len(body) != size {
				t.Fatal("body not preserved")
			}
			w.WriteHeader(204)
		}))
		r := httptest.NewRequest("GET", "/api/health", strings.NewReader(strings.Repeat("x", size)))
		r.ContentLength = -1
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := 204
		if size > int(MaxBodyBytes) {
			want = 413
		}
		if w.Code != want || called != (want == 204) {
			t.Fatal("boundary", size, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/logout", nil)
	r.Body = failedBody{}
	r.ContentLength = -1
	w := httptest.NewRecorder()
	RequestLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("failed body reached endpoint") })).ServeHTTP(w, r)
	if w.Code != 400 || strings.Contains(w.Body.String(), "private-driver-detail") {
		t.Fatal("body error leaked")
	}
}

func TestCORSDeniesAmbiguousHeadersAndNeverFallsBack(t *testing.T) {
	for _, name := range []string{"no-config", "wildcard-config", "empty-origin", "duplicate-origin", "duplicate-method", "duplicate-headers"} {
		t.Run(name, func(t *testing.T) {
			origins := []string{"https://staff.invalid"}
			if name == "no-config" {
				origins = nil
			}
			if name == "wildcard-config" {
				origins = []string{"*"}
			}
			r := httptest.NewRequest("OPTIONS", "/api/login", nil)
			r.Header.Set("Origin", "https://staff.invalid")
			r.Header.Set("Access-Control-Request-Method", "POST")
			r.Header.Set("Access-Control-Request-Headers", "Content-Type")
			switch name {
			case "empty-origin":
				r.Header.Set("Origin", "")
			case "duplicate-origin":
				r.Header.Add("Origin", "https://staff.invalid")
			case "duplicate-method":
				r.Header.Add("Access-Control-Request-Method", "PUT")
			case "duplicate-headers":
				r.Header.Add("Access-Control-Request-Headers", "X-CSRF-Token")
			}
			w := httptest.NewRecorder()
			New(origins, nil).BukaPintuCORS(func(http.ResponseWriter, *http.Request) { t.Fatal("denial reached endpoint") })(w, r)
			if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") == "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("unsafe CORS", w.Code)
			}
		})
	}
}

func TestTrackingLimitCannotBeBypassedByForwardedHeaders(t *testing.T) {
	m := New(nil, nil)
	calls := 0
	h := m.TrackingLimit(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) })
	for i := 0; i < 61; i++ {
		method := "GET"
		if i%2 == 1 {
			method = "HEAD"
		}
		r := httptest.NewRequest(method, "/api/tracking/synthetic", nil)
		r.RemoteAddr = fmt.Sprintf("127.0.0.1:%d", 2000+i)
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i))
		w := httptest.NewRecorder()
		h(w, r)
		want := 204
		if i == 60 {
			want = 429
			if w.Header().Get("Retry-After") == "" {
				t.Error("missing retry")
			}
		}
		if w.Code != want {
			t.Fatalf("attempt=%d status=%d", i, w.Code)
		}
	}
	if calls != 60 {
		t.Fatal("limited request reached use case")
	}
}
