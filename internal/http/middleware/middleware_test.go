package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestInjectedJWTValidation(t *testing.T) {
	const secret = "synthetic-middleware-test-secret"
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
		{name: "missing-server-secret", signingSecret: secret, want: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := tc.header
			if tc.signingSecret != "" {
				expiry := time.Now().Add(time.Hour)
				if tc.expired {
					expiry = time.Now().Add(-time.Hour)
				}
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"exp": expiry.Unix()}).SignedString([]byte(tc.signingSecret))
				if err != nil {
					t.Fatal(err)
				}
				header = "Bearer " + token
			}
			called := false
			h := New(tc.configuredSecret, nil).ValidasiJWT(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", header)
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
		{"preflight-no-origin", "", "OPTIONS", "PUT", "", 403, false},
		{"preflight-method", "https://staff.invalid", "OPTIONS", "DELETE", "", 403, false},
		{"preflight-header", "https://staff.invalid", "OPTIONS", "POST", "X-Untrusted", 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			m := New("fixture", []string{"https://staff.invalid", "https://owner.invalid"})
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
