package http

import (
	"context"
	"encoding/csv"
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

func TestPostgresReportHTTP(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := postgres.New(db)
	at := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	rt := service.Runtime{Clock: service.ClockFunc(func() time.Time { return at }), IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	password := "Frasa uji laporan sintetis 2026"
	hash, err := (platform.Passwords{}).Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	owner := domain.Staff{ID: "report-owner", Username: "report.owner", Name: "Owner Contoh", Role: domain.Owner, Active: true, CreatedAt: at, UpdatedAt: at}
	cashier := owner
	cashier.ID = "report-cashier"
	cashier.Username = "report.cashier"
	cashier.Role = domain.Cashier
	if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
		for _, a := range []domain.Staff{owner, cashier} {
			if err := tx.AddStaff(ctx, a, hash); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c, err := service.NewCustomers(store, rt).Create(ctx, owner.ID, service.CustomerInput{Name: "=Pelanggan CSV"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := service.NewCatalog(store, rt).Configure(ctx, owner.ID, service.ServiceInput{Name: "Cuci", Unit: domain.PerPiece, UnitPrice: 10000, Turnaround: time.Hour, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	o, err := service.NewOrders(store, rt).Create(ctx, owner.ID, service.OrderInput{IdempotencyKey: "create", CustomerID: c.ID, Items: []domain.ItemRequest{{ServiceID: s.Service.ID, Quantity: domain.Quantity{Pieces: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NewPayments(store, rt).Record(ctx, cashier.ID, service.PaymentInput{OrderID: o.Order.ID(), Amount: 4000, Method: domain.QRIS, IdempotencyKey: "pay"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	tokens := security.Tokens{Secret: "synthetic-report-http-jwt-secret-32bytes", Issuer: origin, Audience: "ulaundry-staff", Lifetime: time.Hour}
	manager := &security.Manager{Tokens: tokens, Store: store, Secret: "synthetic-report-http-cookie-key-32bytes", Origin: origin, Secure: true, SameSite: http.SameSiteLaxMode, Limiter: &security.Limiter{}, PeerLimit: 100}
	authRT := rt
	authRT.Clock = platform.SystemClock{}
	use := handler.Dependencies{Sessions: manager, Auth: service.NewAuth(store, authRT, platform.Passwords{}, tokens), Reports: service.NewReports(service.WithAccessAudit(store, store), rt)}
	server.Config.Handler = NewRouter(handler.New(use), middleware.New([]string{origin}, manager), http.FS(fstest.MapFS{}))
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	request := func(method, path, body string, cookie *http.Cookie) (*http.Response, []byte) {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", origin)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
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
	login := func(username string) *http.Cookie {
		t.Helper()
		body, err := json.Marshal(map[string]string{"username": username, "password": password})
		if err != nil {
			t.Fatal(err)
		}
		res, raw := request("POST", "/api/login", string(body), nil)
		if res.StatusCode != 200 || len(res.Cookies()) != 1 {
			t.Fatal("login", res.StatusCode, string(raw))
		}
		return res.Cookies()[0]
	}
	query := "?from=" + url.QueryEscape(at.Add(-time.Minute).Format(time.RFC3339)) + "&to=" + url.QueryEscape(at.Add(time.Minute).Format(time.RFC3339))
	ownerCookie, cashierCookie := login(owner.Username), login(cashier.Username)
	for _, path := range []string{"/api/laporan/dashboard" + query, "/api/laporan/orders" + query, "/api/laporan/ledger" + query + "&format=csv"} {
		for _, denied := range []struct {
			cookie *http.Cookie
			status int
		}{{nil, 401}, {cashierCookie, 403}} {
			res, raw := request("GET", path, "", denied.cookie)
			if res.StatusCode != denied.status || !strings.Contains(res.Header.Get("Content-Type"), "application/json") {
				t.Fatal(path, res.StatusCode, string(raw))
			}
		}
	}
	res, raw := request("GET", "/api/laporan/dashboard"+query, "", ownerCookie)
	var dashboard dto.DashboardResponse
	if err := json.Unmarshal(raw, &dashboard); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || dashboard.Money.Payments != 4000 || dashboard.Orders.Receivable != 6000 || dashboard.Money.UnassignedPayments != 4000 || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("dashboard", res.StatusCode, string(raw))
	}
	if len(dashboard.Popular) != 1 || dashboard.Popular[0].Unit != "item" {
		t.Fatal("public service unit", dashboard.Popular)
	}
	res, raw = request("GET", "/api/laporan/orders"+query+"&kind=receivable&format=csv", "", ownerCookie)
	rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil || res.StatusCode != 200 || len(rows) != 2 || rows[1][1] != "'=Pelanggan CSV" || rows[1][5] != "6000" {
		t.Fatal("CSV", res.StatusCode, rows, err)
	}
	res, raw = request("GET", "/api/laporan/ledger"+query, "", ownerCookie)
	var ledger dto.ReportPage[dto.ReportLedger]
	if err := json.Unmarshal(raw, &ledger); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || len(ledger.Items) != 1 || ledger.Items[0].Amount != 4000 || strings.Contains(string(raw), "idempotency") || strings.Contains(string(raw), "customer_contact") {
		t.Fatal("ledger", string(raw))
	}
	res, raw = request("HEAD", "/api/laporan/ledger"+query+"&format=csv", "", ownerCookie)
	if res.StatusCode != 200 || len(raw) != 0 || res.Header.Get("Content-Length") == "0" {
		t.Fatal("HEAD", res.StatusCode, len(raw))
	}
}
