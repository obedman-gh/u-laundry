package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/service"
	"ulaundry/internal/testutil"
)

type privacyTracking struct{ calls int }

func (s *privacyTracking) Get(_ context.Context, token string) (service.TrackingView, error) {
	s.calls++
	if token[0] == 'A' {
		return service.TrackingView{}, &domain.Error{Kind: domain.NotFound}
	}
	return service.TrackingView{Status: domain.Processing, EstimatedReadyAt: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), Code: "abc123def456", Message: "Pesanan sedang diproses."}, nil
}
func TestTrackingPublicPrivacyAndInvalidTokens(t *testing.T) {
	use := &privacyTracking{}
	h := handler.New(handler.Dependencies{Tracking: use})
	router := NewRouter(h, middleware.New(nil, testutil.TestSessions("synthetic-privacy-secret-32bytes")), http.FS(fstest.MapFS{}))
	for _, token := range []string{"ORDER-1", strings.Repeat("A", 43), strings.Repeat("B", 42) + "A"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/api/tracking/"+token, nil))
		if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Fatal("privacy headers")
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if token[0] != 'B' {
			if w.Code != 404 || len(body) != 1 || !strings.Contains(w.Body.String(), `"code":"not_found"`) || !strings.Contains(w.Body.String(), `"message":"Data tidak ditemukan."`) {
				t.Fatal("invalid/missing token differs", w.Code, w.Body.String())
			}
			continue
		}
		if w.Code != 200 || len(body) != 4 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, key := range []string{"tracking_code", "status", "estimated_ready_at", "message"} {
			if _, ok := body[key]; !ok {
				t.Fatal("projection changed", key)
			}
		}
		for _, private := range []string{token, "customer", "contact", "notes", "ledger", "payment", "\"ready_at\"", "collected_at"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("private field", private)
			}
		}
	}
	if use.calls != 2 {
		t.Fatal("malformed token reached repository/use case")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("HEAD", "/api/tracking/"+strings.Repeat("B", 42)+"A", nil))
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD projection")
	}
}

func TestCustomerAndReceiptAuthorization(t *testing.T) {
	const secret = "synthetic-privacy-secret-32bytes"
	manager := testutil.TestSessions(secret)
	work := &roleWork{role: domain.Role("unknown")}
	orders := service.NewOrders(work, service.Runtime{})
	h := handler.New(handler.Dependencies{Orders: orders, Customers: service.NewCustomers(work, service.Runtime{})})
	router := NewRouter(h, middleware.New(nil, manager), http.FS(fstest.MapFS{}))
	for _, path := range []string{"/api/customers", "/api/customers/customer-1", "/api/customers/customer-1/orders", "/api/pesanan/order-1", "/api/pesanan/order-1/struk"} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 401 {
				t.Fatal("anonymous private read", path, w.Code)
			}
		}
	}
	// A valid cookie cannot bypass current staff authorization in the use case.
	token, err := manager.Tokens.Sign(domain.Staff{ID: "actor-1", Username: "staff.test", Role: domain.Cashier}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/pesanan/order-1/struk", "/api/customers", "/api/customers/customer-1/orders"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 403 || strings.Contains(w.Body.String(), "<html") {
			t.Fatal("receipt/customer role bypass", path, w.Code)
		}
	}
	for _, path := range []string{"/api/pesanan/order-1/tracking", "/api/customers"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{"version":1}`))
		r.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: token})
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("mutation missing CSRF accepted", w.Code)
		}
	}
	// A public receipt route was deliberately not added.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/tracking/"+strings.Repeat("A", 43)+"/struk", nil))
	if w.Code != 404 {
		t.Fatal("public receipt exposed")
	}
}
