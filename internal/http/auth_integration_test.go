package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/security"
	"ulaundry/internal/service"
	"ulaundry/internal/testutil"
	"ulaundry/migrations"
)

func TestPostgresCookieAuthenticationLifecycle(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Reproduce version 4, preserve a legacy row, then apply only the new upgrade.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `CREATE TABLE public.schema_migrations(version INTEGER PRIMARY KEY,name TEXT UNIQUE NOT NULL,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	for i, name := range names[:4] {
		body, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO public.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, i+1, name, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO public.pesanan(id_pesanan,nama_customer,layanan,total_biaya,status) VALUES('AUTH-LEGACY','Synthetic','Cuci',1000,'Diproses')`); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Up(ctx, db); err != nil {
		t.Fatal("migration repeat failed", err)
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil || count != 6 {
		t.Fatal("upgrade history", err)
	}
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM pesanan WHERE id_pesanan='AUTH-LEGACY' AND total_biaya=1000`).Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy data lost", err)
	}
	hash, err := (platform.Passwords{}).Hash("Frasa sandi sintetis 2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO public.staff(id,username,name,role,active,password_hash,created_at,updated_at) VALUES('staff-auth','staff.test','Synthetic Cashier','cashier',true,$1,now(),now())`, hash); err != nil {
		t.Fatal(err)
	}
	store := postgres.New(db)
	runtime := service.Runtime{Clock: platform.SystemClock{}, IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	tokens := security.Tokens{Secret: "synthetic-integration-jwt-key-32bytes", Issuer: origin, Audience: "ulaundry-staff", Lifetime: 8 * time.Hour}
	manager := &security.Manager{Tokens: tokens, Store: store, Secret: "synthetic-integration-cookie-key-32bytes", Origin: origin, Secure: true, SameSite: http.SameSiteLaxMode, Limiter: &security.Limiter{}, PeerLimit: 100}
	use := handler.Dependencies{Auth: service.NewAuth(store, runtime, platform.Passwords{}, tokens), Sessions: manager}
	server.Config.Handler = NewRouter(handler.New(use), middleware.New([]string{origin}, manager), http.FS(fstest.MapFS{}))
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	request := func(method, path, body, csrf string) (int, map[string]any, []*http.Cookie) {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", origin)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal("HTTP request failed", err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err = json.Unmarshal(raw, &data); err != nil {
			t.Fatal("non-JSON response", err)
		}
		return response.StatusCode, data, response.Cookies()
	}
	login := func() (string, *http.Cookie) {
		t.Helper()
		status, data, cookies := request("POST", "/api/login", `{"username":"staff.test","password":"Frasa sandi sintetis 2026"}`, "")
		if status != 200 || data["token"] != nil || len(cookies) != 1 {
			t.Fatalf("cookie login failed: status=%d", status)
		}
		csrf, ok := data["csrf_token"].(string)
		if !ok {
			t.Fatal("missing CSRF")
		}
		return csrf, cookies[0]
	}
	// Unknown/wrong password have the same public failure and no cookies.
	var failure string
	for _, body := range []string{`{"username":"missing","password":"incorrect"}`, `{"username":"staff.test","password":"incorrect"}`} {
		status, data, cookies := request("POST", "/api/login", body, "")
		if status != 401 || len(cookies) != 0 {
			t.Fatal("generic failure")
		}
		message := data["error"].(map[string]any)["message"].(string)
		if failure != "" && failure != message {
			t.Fatal("username enumeration")
		}
		failure = message
	}
	csrf, cookie := login()
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Name != "__Host-ulaundry_session" {
		t.Fatal("unsafe cookie")
	}
	status, data, _ := request("GET", "/api/session", "", "")
	if status != 200 || data["role"] != "cashier" || data["user_id"] != "staff-auth" {
		t.Fatal("cashier session lost")
	}
	// A replacement manager reads the same session from PostgreSQL.
	restarted := *manager
	replay := httptest.NewRequest("GET", origin+"/api/session", nil)
	replay.AddCookie(cookie)
	if _, err := restarted.Authenticate(replay); err != nil {
		t.Fatal("session not persistent", err)
	}
	status, _, _ = request("POST", "/api/logout", "", "")
	if status != 403 {
		t.Fatal("logout CSRF bypass")
	}
	status, _, removed := request("POST", "/api/logout", "", csrf)
	if status != 200 || len(removed) != 1 || removed[0].MaxAge != -1 {
		t.Fatal("logout failed")
	}
	if _, err := restarted.Authenticate(replay); err == nil {
		t.Fatal("old cookie replay after logout")
	}
	status, _, _ = request("GET", "/api/session", "", "")
	if status != 401 {
		t.Fatal("logout retained access")
	}
	// Staff changes revoke permanently, even if the role is restored.
	_, cookie = login()
	replay = httptest.NewRequest("GET", origin+"/api/session", nil)
	replay.AddCookie(cookie)
	for _, role := range []string{"admin", "cashier"} {
		if _, err = db.ExecContext(ctx, `UPDATE staff SET role=$1 WHERE id='staff-auth'`, role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.Authenticate(replay); err == nil {
		t.Fatal("role change failed to revoke")
	}
	_, cookie = login()
	replay = httptest.NewRequest("GET", origin+"/api/session", nil)
	replay.AddCookie(cookie)
	if _, err = db.ExecContext(ctx, `UPDATE staff SET active=false WHERE id='staff-auth'`); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(replay); err == nil {
		t.Fatal("inactive user retained session")
	}
	if _, err = db.ExecContext(ctx, `UPDATE staff SET active=true WHERE id='staff-auth'`); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(replay); err == nil {
		t.Fatal("reactivation restored revoked cookie")
	}

	// Password reset revokes permanently; stale credential snapshots cannot start
	// a session even when signed by the issuer before the password changed.
	_, cookie = login()
	replay = httptest.NewRequest("GET", origin+"/api/session", nil)
	replay.AddCookie(cookie)
	changedHash, err := (platform.Passwords{}).Hash("Frasa pengganti sintetis 2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE staff SET password_hash=$1 WHERE id='staff-auth'`, changedHash); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(replay); err == nil {
		t.Fatal("password change retained session")
	}
	stale, err := tokens.Sign(domain.Staff{ID: "staff-auth", Username: "staff.test", Role: domain.Cashier}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(ctx, stale, sha256.Sum256([]byte(hash)), "request-stale"); err == nil {
		t.Fatal("stale credential snapshot created session")
	}
	if _, err = db.ExecContext(ctx, `UPDATE staff SET password_hash=$1 WHERE id='staff-auth'`, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(replay); err == nil {
		t.Fatal("restoring old hash revived session")
	}

	// Audit failure rolls back session creation and never emits a cookie.
	if _, err = db.ExecContext(ctx, `CREATE FUNCTION public.test_reject_auth_success() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='login_success' THEN RAISE EXCEPTION 'synthetic auth audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_auth_success BEFORE INSERT ON auth_events FOR EACH ROW EXECUTE FUNCTION public.test_reject_auth_success()`); err != nil {
		t.Fatal(err)
	}
	var before int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM staff_sessions`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	status, _, cookies := request("POST", "/api/login", `{"username":"staff.test","password":"Frasa sandi sintetis 2026"}`, "")
	if status < 500 || len(cookies) != 0 {
		t.Fatal("audit failure emitted session")
	}
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM staff_sessions`).Scan(&count); err != nil || count != before {
		t.Fatal("session audit not atomic", err)
	}
	var failures, successes, logouts int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE kind='login_failure' AND actor_id IS NULL),count(*) FILTER(WHERE kind='login_success'),count(*) FILTER(WHERE kind='logout') FROM auth_events`).Scan(&failures, &successes, &logouts); err != nil || failures != 2 || successes != 4 || logouts != 1 {
		t.Fatal("auth audit mismatch", err)
	}
	var audit []byte
	if err = db.QueryRowContext(ctx, `SELECT coalesce(json_agg(auth_events)::text,'[]') FROM auth_events`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"Frasa sandi sintetis 2026", hash, cookie.Value, "missing"} {
		if bytes.Contains(audit, []byte(secret)) {
			t.Fatal("credentials or submitted identifier in audit")
		}
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM auth_events`); err == nil {
		t.Fatal("auth audit not append-only")
	}
}
