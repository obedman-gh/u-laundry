package security_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/security"
	"ulaundry/internal/testutil"
)

const key = "synthetic-security-jwt-key-32bytes-0001"

var fixed = time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)

func policy() security.Tokens {
	return security.Tokens{Secret: key, Issuer: "https://laundry.invalid", Audience: "ulaundry-staff", Lifetime: 8 * time.Hour}
}
func claims() jwt.MapClaims {
	return jwt.MapClaims{"sub": "staff-1", "username": "staff.test", "role": "cashier", "iss": "https://laundry.invalid", "aud": "ulaundry-staff", "iat": fixed.Unix(), "exp": fixed.Add(8 * time.Hour).Unix(), "jti": "synthetic-session-identifier-0001"}
}
func TestTokenClaimsPolicy(t *testing.T) {
	for _, name := range []string{"valid", "expired", "wrong-algorithm", "wrong-issuer", "wrong-audience", "missing-issuer", "missing-audience", "missing-iat", "future-iat", "missing-exp", "inverted-times", "long-lifetime", "missing-sub", "invalid-role", "missing-jti", "wrong-signature", "missing-secret"} {
		t.Run(name, func(t *testing.T) {
			c := claims()
			method := jwt.SigningMethodHS256
			signKey := key
			p := policy()
			switch name {
			case "expired":
				c["iat"] = fixed.Add(-9 * time.Hour).Unix()
				c["exp"] = fixed.Add(-time.Hour).Unix()
			case "wrong-algorithm":
				method = jwt.SigningMethodHS384
			case "wrong-issuer":
				c["iss"] = "https://other.invalid"
			case "wrong-audience":
				c["aud"] = "other"
			case "missing-issuer":
				delete(c, "iss")
			case "missing-audience":
				delete(c, "aud")
			case "missing-iat":
				delete(c, "iat")
			case "future-iat":
				c["iat"] = fixed.Add(time.Minute).Unix()
			case "missing-exp":
				delete(c, "exp")
			case "inverted-times":
				c["iat"] = fixed.Add(-time.Hour).Unix()
				c["exp"] = fixed.Add(-2 * time.Hour).Unix()
			case "long-lifetime":
				c["exp"] = fixed.Add(13 * time.Hour).Unix()
			case "missing-sub":
				delete(c, "sub")
			case "invalid-role":
				c["role"] = "superuser"
			case "missing-jti":
				delete(c, "jti")
			case "wrong-signature":
				signKey = "different-synthetic-key"
			case "missing-secret":
				p.Secret = ""
			}
			raw, err := jwt.NewWithClaims(method, c).SignedString([]byte(signKey))
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := p.Parse(raw, fixed)
			if name == "valid" {
				if err != nil || parsed.Subject != "staff-1" || parsed.Role != domain.Cashier {
					t.Fatal("valid token rejected")
				}
			} else if err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}
func TestSessionCookieCSRFAndLogout(t *testing.T) {
	store := &testutil.SessionStore{}
	m := &security.Manager{Tokens: policy(), Store: store, Secret: "synthetic-csrf-key-32bytes-0000001", Origin: "https://laundry.invalid", Secure: true, SameSite: http.SameSiteLaxMode, Now: func() time.Time { return fixed }, Limiter: &security.Limiter{}}
	raw, err := m.Tokens.Sign(domain.Staff{ID: "staff-1", Username: "staff.test", Role: domain.Cashier}, fixed)
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Start(context.Background(), raw, sha256.Sum256([]byte("synthetic-hash")), "request-1")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	m.SetCookie(w, raw, c)
	cookie := w.Result().Cookies()[0]
	if cookie.Name != "__Host-ulaundry_session" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge != 8*3600 {
		t.Fatal("unsafe cookie flags")
	}
	r := httptest.NewRequest("POST", "https://laundry.invalid/api/logout", nil)
	r.AddCookie(cookie)
	r.Header.Set("Origin", m.Origin)
	r.Header.Set("X-CSRF-Token", m.CSRF(c))
	if _, err := m.Authenticate(r); err != nil || !m.VerifyCSRF(r, c) {
		t.Fatal("valid session/CSRF rejected")
	}
	for _, name := range []string{"missing", "wrong", "duplicate", "cross-origin", "cross-site"} {
		t.Run(name, func(t *testing.T) {
			bad := r.Clone(context.Background())
			bad.Header = r.Header.Clone()
			switch name {
			case "missing":
				bad.Header.Del("X-CSRF-Token")
			case "wrong":
				bad.Header.Set("X-CSRF-Token", strings.Repeat("x", 43))
			case "duplicate":
				bad.Header.Add("X-CSRF-Token", m.CSRF(c))
			case "cross-origin":
				bad.Header.Set("Origin", "https://other.invalid")
			case "cross-site":
				bad.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			if m.VerifyCSRF(bad, c) {
				t.Fatal("CSRF bypass")
			}
		})
	}
	// A fresh manager instance can read the server-backed session (no local map).
	restarted := *m
	if _, err := restarted.Authenticate(r); err != nil {
		t.Fatal("session depends on manager process state")
	}
	if err := m.Logout(context.Background(), c, "request-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Authenticate(r); !errors.Is(err, security.ErrUnauthorized) {
		t.Fatal("logout did not revoke")
	}
	w = httptest.NewRecorder()
	m.ClearCookie(w)
	removed := w.Result().Cookies()[0]
	if removed.MaxAge != -1 || !removed.HttpOnly || !removed.Secure || removed.Name != cookie.Name {
		t.Fatal("unsafe cookie deletion")
	}
	data, _ := json.Marshal(store.Events)
	if strings.Contains(string(data), raw) || strings.Contains(string(data), m.CSRF(c)) || strings.Contains(string(data), "synthetic-hash") {
		t.Fatal("secret entered audit")
	}
}
func TestLimiterConcurrentBoundAndRecovery(t *testing.T) {
	limiter := &security.Limiter{}
	var allowed atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 40; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			ok, _, _ := limiter.Take(fixed, security.Limit{Key: "peer", Count: 5, Window: time.Minute})
			if ok {
				allowed.Add(1)
			}
		}()
	}
	group.Wait()
	if allowed.Load() != 5 {
		t.Fatal("concurrent limit bypass")
	}
	if ok, retry, _ := limiter.Take(fixed, security.Limit{Key: "peer", Count: 5, Window: time.Minute}); ok || retry <= 0 {
		t.Fatal("missing retry limit")
	}
	if ok, _, _ := limiter.Take(fixed.Add(time.Minute), security.Limit{Key: "peer", Count: 5, Window: time.Minute}); !ok {
		t.Fatal("limit did not expire")
	}
	bounded := &security.Limiter{Capacity: 1}
	bounded.Take(fixed, security.Limit{Key: "one", Count: 1, Window: time.Minute})
	if ok, _, _ := bounded.Take(fixed, security.Limit{Key: "two", Count: 1, Window: time.Minute}); ok {
		t.Fatal("capacity evicted live bucket")
	}
}
func TestPeerLimitIgnoresSpoofedForwardingHeaders(t *testing.T) {
	store := &testutil.SessionStore{}
	m := &security.Manager{Store: store, Limiter: &security.Limiter{}, Secret: key, Now: func() time.Time { return fixed }, PeerLimit: 2, PeerWindow: time.Minute}
	r := httptest.NewRequest("POST", "https://laundry.invalid/api/login", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	for i := 0; i < 4; i++ {
		r.Header.Set("X-Forwarded-For", strings.Repeat("1", i+1))
		ok, retry, err := m.AllowLogin(context.Background(), r, "staff.test", "request-1")
		if err != nil {
			t.Fatal(err)
		}
		if ok != (i < 2) || (i >= 2 && retry <= 0) {
			t.Fatal("spoofed peer bypassed limit")
		}
	}
	if len(store.Events) != 1 || store.Events[0].Kind != "login_limited" {
		t.Fatal("rate-limit events not bounded")
	}
}

func TestAccountAndGlobalLoginLimits(t *testing.T) {
	for _, kind := range []string{"account", "global"} {
		t.Run(kind, func(t *testing.T) {
			m := &security.Manager{Store: &testutil.SessionStore{}, Limiter: &security.Limiter{}, Secret: key, Now: func() time.Time { return fixed }, PeerLimit: 100, PeerWindow: time.Minute}
			limit := 10
			if kind == "global" {
				limit = 100
			}
			for i := 0; i <= limit; i++ {
				r := httptest.NewRequest("POST", "https://laundry.invalid/api/login", nil)
				r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", i+1)
				username := "staff.test"
				if kind == "global" {
					username = fmt.Sprintf("staff-%d", i)
				}
				allowed, _, err := m.AllowLogin(context.Background(), r, username, "request-1")
				if err != nil {
					t.Fatal(err)
				}
				if allowed != (i < limit) {
					t.Fatalf("%s login limit bypass", kind)
				}
			}
		})
	}
}
