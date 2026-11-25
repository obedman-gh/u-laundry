package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/security"
	"ulaundry/internal/service"
	"ulaundry/internal/testutil"
)

func TestPostgresAuditHTTP(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := postgres.New(db)
	uow := service.WithAccessAudit(store, store)
	rt := service.Runtime{Clock: platform.SystemClock{}, IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	password := "Frasa uji HTTPS audit sintetis 2026"
	hash, err := (platform.Passwords{}).Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
		for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
			a := domain.Staff{ID: string(role), Username: "audit." + string(role), Name: "Staf Contoh", Role: role, Active: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if err := tx.AddStaff(ctx, a, hash); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	tokens := security.Tokens{Secret: "synthetic-audit-http-signing-secret-32bytes", Issuer: origin, Audience: "ulaundry-staff", Lifetime: time.Hour}
	manager := &security.Manager{Tokens: tokens, Store: store, Secret: "synthetic-audit-http-cookie-secret-32bytes", Origin: origin, Secure: true, SameSite: http.SameSiteLaxMode, Limiter: &security.Limiter{}, PeerLimit: 100}
	use := handler.Dependencies{Sessions: manager, Auth: service.NewAuth(uow, rt, platform.Passwords{}, tokens), Audit: service.NewAuditTrail(uow, rt), Staff: service.NewStaffManagement(uow, rt, platform.Passwords{}), Catalog: service.NewCatalog(uow, rt)}
	server.Config.Handler = NewRouter(handler.New(use), middleware.New([]string{origin}, manager), http.FS(fstest.MapFS{}))
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	request := func(method, path, body, csrf string, cookie *http.Cookie) (*http.Response, []byte) {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Request-ID", "caller-controlled")
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(res.Body)
		if closeErr := res.Body.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil {
			t.Fatal(err)
		}
		return res, raw
	}
	type loginSession struct {
		cookie *http.Cookie
		csrf   string
	}
	login := func(role string) loginSession {
		t.Helper()
		body, err := json.Marshal(map[string]string{"username": "audit." + role, "password": password})
		if err != nil {
			t.Fatal(err)
		}
		res, raw := request("POST", "/api/login", string(body), "", nil)
		var v dto.LoginResponse
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 || len(res.Cookies()) != 1 || v.CSRFToken == "" {
			t.Fatal("login", res.StatusCode, string(raw))
		}
		return loginSession{res.Cookies()[0], v.CSRFToken}
	}
	owner, admin, cashier := login("owner"), login("admin"), login("cashier")
	for _, path := range []string{"/api/audit/events", "/api/audit?entity=staff&entity_id=owner"} {
		for _, tc := range []struct {
			cookie *http.Cookie
			status int
		}{{nil, 401}, {admin.cookie, 403}, {cashier.cookie, 403}, {owner.cookie, 200}} {
			res, raw := request("GET", path, "", "", tc.cookie)
			if res.StatusCode != tc.status || res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal(path, res.StatusCode, string(raw))
			}
		}
	}
	res, raw := request("POST", "/api/services", `{"name":"Nama konfigurasi privat","unit":"item","unit_price":12300,"turnaround_minutes":60,"active":true,"display_order":0}`, owner.csrf, owner.cookie)
	if res.StatusCode != 201 {
		t.Fatal("configuration", res.StatusCode, string(raw))
	}
	rid := res.Header.Get("X-Request-ID")
	if rid == "caller-controlled" || !domain.ValidAuditRequestID(rid) {
		t.Fatal("caller controlled audit correlation")
	}
	res, raw = request("GET", "/api/audit/events?action=service_configured&request_id="+url.QueryEscape(rid), "", "", owner.cookie)
	var v dto.AuditHistoryResponse
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || len(v.Items) != 1 || v.Items[0].RequestID != rid || v.Items[0].ActorID != "owner" || v.Items[0].Metadata.UnitPrice == nil || *v.Items[0].Metadata.UnitPrice != 12300 || v.Items[0].Metadata.Version != 1 {
		t.Fatal("correlation/metadata", string(raw))
	}
	for _, private := range []string{"Nama konfigurasi privat", password, hash, owner.cookie.Value, "caller-controlled"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private payload entered audit")
		}
	}
	res, raw = request("GET", "/api/audit/events?source=security&action=login_success", "", "", owner.cookie)
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || len(v.Items) != 3 {
		t.Fatal("security union", string(raw))
	}
	for _, e := range v.Items {
		if e.Entity != "staff" || e.EntityID != e.ActorID || e.RequestID == "" {
			t.Fatal("auth target/correlation", e)
		}
	}
	res, raw = request("HEAD", "/api/audit/events", "", "", owner.cookie)
	if res.StatusCode != 200 || len(raw) != 0 {
		t.Fatal("HEAD audit", res.StatusCode)
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		res, raw = request(method, "/api/audit/events", `{}`, cashier.csrf, cashier.cookie)
		if res.StatusCode != 405 {
			t.Fatal("audit mutation endpoint exposed", method, res.StatusCode, string(raw))
		}
	}
}
