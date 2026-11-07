package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"ulaundry/internal/http/response"
	"ulaundry/internal/testutil"
)

func TestRequestContextRecoveryAndCorrelation(t *testing.T) {
	h := RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if response.RequestID(r.Context()) == "" {
			t.Fatal("missing context identity")
		}
		panic("private-panic-secret")
	}))
	r := httptest.NewRequest("GET", "/api/test", nil)
	r.Header.Set("X-Request-ID", "private-caller-id")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var body response.ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 || body.Error.Code != "internal_error" || body.Error.RequestID != w.Header().Get("X-Request-ID") || body.Error.RequestID == "private-caller-id" || strings.Contains(w.Body.String(), "private-") {
		t.Fatal("panic/caller details leaked")
	}
}
func TestRequestContextAbortsAfterPartialResponse(t *testing.T) {
	defer func() {
		if value := recover(); value != http.ErrAbortHandler {
			t.Fatalf("want safe connection abort, got %v", value)
		}
	}()
	h := RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201); panic("private-panic-secret") }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/test", nil))
}
func TestDuplicateAuthenticationAndOriginHeaders(t *testing.T) {
	m := New([]string{"https://staff.invalid"}, testutil.TestSessions("synthetic"))
	for _, kind := range []string{"Origin", "Authorization"} {
		t.Run(kind, func(t *testing.T) {
			never := func(http.ResponseWriter, *http.Request) { t.Fatal("ambiguous headers reached endpoint") }
			h := http.Handler(m.ValidasiJWT(never))
			if kind == "Origin" {
				h = m.BukaPintuCORS(never)
			}
			r := httptest.NewRequest("GET", "/api/test", nil)
			value := "Bearer synthetic"
			want := 401
			if kind == "Origin" {
				value = "https://staff.invalid"
				want = 403
			}
			r.Header.Add(kind, value)
			r.Header.Add(kind, value)
			w := httptest.NewRecorder()
			RequestContext(h).ServeHTTP(w, r)
			var body response.ErrorBody
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != want || body.Error.RequestID == "" {
				t.Fatal("invalid duplicate-header error")
			}
		})
	}
}
