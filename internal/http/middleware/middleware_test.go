package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/testutil"

	"github.com/golang-jwt/jwt/v5"
)

func TestInjectedJWTValidation(t *testing.T) {
	const secret = "synthetic-middleware-test-secret-32bytes"
	// A process environment value must not override constructor dependencies.
	t.Setenv("JWT_SECRET", "different-process-secret")
	for _, tc := range []struct {
		name, header, signingSecret, configuredSecret string
		expired                                       bool
		want                                          int
	}{
		{name: "valid", signingSecret: secret, configuredSecret: secret, want: http.StatusNoContent},
		{name: "wrong-signature", signingSecret: "wrong", configuredSecret: secret, want: http.StatusUnauthorized},
		{name: "expired", signingSecret: secret, configuredSecret: secret, expired: true, want: http.StatusUnauthorized},
		{name: "missing", configuredSecret: secret, want: http.StatusUnauthorized},
		{name: "malformed", header: "Bearer malformed", configuredSecret: secret, want: http.StatusUnauthorized},
		{name: "missing-server-secret", signingSecret: secret, want: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := tc.header
			if tc.signingSecret != "" {
				expiry := time.Now().Add(time.Hour)
				if tc.expired {
					expiry = time.Now().Add(-time.Hour)
				}
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, func() jwt.MapClaims { c := testutil.TestClaims("staff-fixture"); c["exp"] = expiry.Unix(); return c }()).SignedString([]byte(tc.signingSecret))
				if err != nil {
					t.Fatal(err)
				}
				header = "Bearer " + token
			}
			called := false
			h := New(nil, testutil.TestSessions(tc.configuredSecret)).ValidasiJWT(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest("GET", "/", nil)
			if header != "" {
				req.AddCookie(&http.Cookie{Name: "ulaundry_session", Value: strings.TrimPrefix(header, "Bearer ")})
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want || called != (tc.want == http.StatusNoContent) {
				t.Fatalf("status=%d called=%v; want status=%d", rr.Code, called, tc.want)
			}
		})
	}
}

func TestTrustedOrigins(t *testing.T) {
	for _, tc := range []struct {
		name, origin, method, requestedMethod, requestedHeaders string
		want                                                    int
		called                                                  bool
	}{
		{"no-origin", "", "GET", "", "", 200, true},
		{"trusted", "https://staff.invalid", "POST", "", "", 200, true},
		{"second-trusted", "https://owner.invalid", "GET", "", "", 200, true},
		{"untrusted", "https://attacker.invalid", "POST", "", "", 403, false},
		{"suffix-attack", "https://staff.invalid.attacker.invalid", "POST", "", "", 403, false},
		{"null-origin", "null", "GET", "", "", 403, false},
		{"valid-preflight", "https://staff.invalid", "OPTIONS", "PUT", "authorization, CONTENT-TYPE", 204, false},
		{"idempotency-preflight", "https://staff.invalid", "OPTIONS", "POST", "authorization, Idempotency-Key", 204, false},
		{"preflight-no-origin", "", "OPTIONS", "PUT", "", 403, false},
		{"preflight-method", "https://staff.invalid", "OPTIONS", "DELETE", "", 403, false},
		{"preflight-header", "https://staff.invalid", "OPTIONS", "POST", "X-Untrusted", 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			m := New([]string{"https://staff.invalid", "https://owner.invalid"}, testutil.TestSessions("fixture"))
			h := m.BukaPintuCORS(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) })
			req := httptest.NewRequest(tc.method, "/api/login", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", tc.requestedMethod)
			req.Header.Set("Access-Control-Request-Headers", tc.requestedHeaders)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want || called != tc.called {
				t.Fatalf("status=%d called=%v", rr.Code, called)
			}
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got == "*" || (tc.want < 400 && got != tc.origin) || (tc.want == 403 && tc.name != "preflight-method" && tc.name != "preflight-header" && got != "") {
				t.Fatalf("unsafe origin header: %q", got)
			}
			if rr.Header().Get("Vary") != "Origin" {
				t.Fatal("origin cache variance missing")
			}
		})
	}
}

func TestJWTRejectsOtherAlgorithmsAndMissingIdentity(t *testing.T) {
	const secret = "synthetic-middleware-test-secret-32bytes"
	for _, tc := range []struct {
		method jwt.SigningMethod
		claims jwt.MapClaims
	}{
		{jwt.SigningMethodHS384, jwt.MapClaims{"sub": "staff-1", "exp": time.Now().Add(time.Hour).Unix()}},
		{jwt.SigningMethodHS256, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()}},
		{jwt.SigningMethodHS256, jwt.MapClaims{"sub": "staff-1"}},
	} {
		token, err := jwt.NewWithClaims(tc.method, tc.claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		h := New(nil, testutil.TestSessions(secret)).ValidasiJWT(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid token reached workflow") })
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: "ulaundry_session", Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatal("token accepted", w.Code)
		}
	}
}
