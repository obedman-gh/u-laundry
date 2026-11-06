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
			h := New(tc.configuredSecret, "").ValidasiJWT(func(w http.ResponseWriter, r *http.Request) {
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
