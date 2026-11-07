package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/security"
	"ulaundry/internal/service"
	"ulaundry/internal/testutil"
)

func TestLoginCookieGenericFailureAndThrottle(t *testing.T) {
	manager := testutil.TestSessions("synthetic-auth-handler-key-32bytes")
	store := &testutil.SessionStore{}
	manager.Store = store
	manager.PeerLimit = 2
	calls := 0
	h := New(Dependencies{Sessions: manager, Auth: fakeLogin(func(context.Context, string, string) (service.LoginResult, error) {
		calls++
		return service.LoginResult{}, service.ErrAuthentication
	})})
	run := func(user string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"`+user+`","password":"synthetic"}`))
		r.Header.Set("Origin", manager.Origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		middleware.RequestContext(http.HandlerFunc(h.Login)).ServeHTTP(w, r)
		return w
	}
	for _, user := range []string{"missing", "existing"} {
		w := run(user)
		checkError(t, w, 401, "unauthorized")
		if len(w.Result().Cookies()) != 0 || strings.Contains(w.Body.String(), user) {
			t.Fatal("failure exposed identity/cookie")
		}
	}
	w := run("existing")
	checkError(t, w, 429, "rate_limited")
	if calls != 2 || w.Header().Get("Retry-After") == "" {
		t.Fatal("brute-force admission not enforced")
	}
	if len(store.Events) != 3 || store.Events[0].Kind != "login_failure" || store.Events[1].Kind != "login_failure" || store.Events[2].Kind != "login_limited" {
		t.Fatal("failure audit missing")
	}
}
func TestSuccessfulLoginNeverReturnsJWT(t *testing.T) {
	manager := testutil.TestSessions("synthetic-auth-handler-key-32bytes")
	manager.Secure = true
	store := &testutil.SessionStore{}
	manager.Store = store
	raw, err := manager.Tokens.Sign(domain.Staff{ID: "staff-1", Username: "staff.test", Role: domain.Cashier}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	h := New(Dependencies{Sessions: manager, Auth: fakeLogin(func(context.Context, string, string) (service.LoginResult, error) {
		return service.LoginResult{Token: raw, Name: "Staf", CredentialFingerprint: sha256.Sum256([]byte("hash"))}, nil
	})})
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"staff.test","password":"synthetic"}`))
	r.Header.Set("Origin", manager.Origin)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	middleware.RequestContext(http.HandlerFunc(h.Login)).ServeHTTP(w, r)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body["token"] != nil || body["csrf_token"] == nil || strings.Contains(w.Body.String(), raw) {
		t.Fatal("JWT exposed or login failed")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.Value != raw {
		t.Fatal("missing secure session cookie")
	}
	// No cookie is emitted if the persistence/audit transaction fails.
	manager.Store = &testutil.SessionStore{Failure: security.ErrUnavailable}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"staff.test","password":"synthetic"}`))
	r.Header.Set("Origin", manager.Origin)
	r.Header.Set("Content-Type", "application/json")
	middleware.RequestContext(http.HandlerFunc(h.Login)).ServeHTTP(w, r)
	if w.Code < 500 || len(w.Result().Cookies()) != 0 {
		t.Fatal("uncommitted session cookie emitted")
	}
}
