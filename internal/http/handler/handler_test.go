package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
	"ulaundry/internal/testutil"
)

type fakeOrders struct {
	OrderUseCase
	create func(context.Context, string, service.OrderInput) (service.CreatedOrder, error)
	list   func(context.Context, string, repository.OrderFilter) ([]service.OrderListItem, error)
	get    func(context.Context, string, string) (repository.OrderRecord, error)
}

func (f fakeOrders) Create(c context.Context, a string, in service.OrderInput) (service.CreatedOrder, error) {
	return f.create(c, a, in)
}
func (f fakeOrders) List(c context.Context, a string, in repository.OrderFilter) ([]service.OrderListItem, error) {
	return f.list(c, a, in)
}
func (f fakeOrders) Get(c context.Context, a, id string) (repository.OrderRecord, error) {
	return f.get(c, a, id)
}
func protected(t *testing.T, method, path, body string, fn http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	const secret = "synthetic-handler-signing-key-32bytes"
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, testutil.TestClaims("actor-1")).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	manager := testutil.TestSessions(secret)
	r.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: token})
	c, err := manager.Tokens.Parse(token, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", manager.Origin)
	r.Header.Set("X-CSRF-Token", manager.CSRF(c))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	r.SetPathValue("id", "order-1")
	w := httptest.NewRecorder()
	middleware.New(nil, manager).ValidasiJWT(fn).ServeHTTP(w, r)
	return w
}
func sampleOrder(t *testing.T) domain.Order {
	t.Helper()
	at := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
	c, err := domain.NewCustomer("c-1", "<script>private</script>", "", "", at)
	if err != nil {
		t.Fatal(err)
	}
	s := domain.Service{ID: "s-1", Name: "Cuci", Unit: domain.PerKilogram, UnitPrice: 7000, Turnaround: time.Hour, Active: true, CreatedAt: at, UpdatedAt: at}
	a := domain.Staff{ID: "actor-1", Username: "staff.test", Name: "Staf", Role: domain.Cashier, Active: true, CreatedAt: at, UpdatedAt: at}
	o, err := domain.NewOrder(domain.NewOrderInput{ID: "order-1", CreatedAt: at, Items: []domain.ItemRequest{{ServiceID: s.ID, Quantity: domain.Quantity{Grams: 2500}}}}, c, []domain.Service{s}, a)
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func TestCreateOrderDelegatesAndRejectsUntrustedTotals(t *testing.T) {
	calls := 0
	o := sampleOrder(t)
	h := New(Dependencies{Orders: fakeOrders{create: func(ctx context.Context, actor string, in service.OrderInput) (service.CreatedOrder, error) {
		calls++
		if actor != "actor-1" || in.CustomerID != "c-1" || len(in.Items) != 1 || in.Items[0].Quantity.Grams != 2500 {
			t.Fatal("request/identity not delegated")
		}
		return service.CreatedOrder{Order: o, TrackingToken: "one-time-token"}, nil
	}}})
	w := protected(t, "POST", "/api/pesanan", `{"customer_id":"c-1","items":[{"service_id":"s-1","grams":2500}]}`, h.BikinPesananBaru)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"total_biaya":17500`) || !strings.Contains(w.Body.String(), `"tracking_token":"one-time-token"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{`{"customer_id":"c-1","total_biaya":1}`, `{"customer_id":"c-1","role":"owner"}`, `{"customer_id":"c-1","items":[{"service_id":"s-1","grams":2500,"unit_price":1}]}`} {
		w = protected(t, "POST", "/api/pesanan", body, h.BikinPesananBaru)
		if w.Code != http.StatusBadRequest || calls != 1 {
			t.Fatal("untrusted fields reached workflow", w.Code, w.Body.String())
		}
	}
}
func TestListPassesStatusPaginationActorAndContext(t *testing.T) {
	h := New(Dependencies{Orders: fakeOrders{list: func(ctx context.Context, actor string, f repository.OrderFilter) ([]service.OrderListItem, error) {
		if actor != "actor-1" || middleware.ActorID(ctx) != actor || f.Status != domain.Processing || f.Page.Limit != 10 || f.Page.Offset != 20 {
			t.Fatal("filter/identity/context lost")
		}
		return nil, nil
	}}})
	w := protected(t, "GET", "/api/pesanan/status?q=processing&limit=10&offset=20", "", h.GetPesananByStatus)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestServiceFailuresAreRedactedAndReceiptIsEscaped(t *testing.T) {
	h := New(Dependencies{Orders: fakeOrders{get: func(context.Context, string, string) (repository.OrderRecord, error) {
		return repository.OrderRecord{}, errors.New("private-postgres-secret")
	}}})
	w := protected(t, "GET", "/api/pesanan/order-1", "", h.GetPesananById)
	if w.Code != 500 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("internal failure leaked")
	}
	o := sampleOrder(t)
	h = New(Dependencies{Orders: fakeOrders{get: func(_ context.Context, actor, id string) (repository.OrderRecord, error) {
		if actor != "actor-1" || id != "order-1" {
			t.Fatal("receipt identity missing")
		}
		return repository.OrderRecord{Order: o, Version: 1}, nil
	}}})
	w = protected(t, "GET", "/api/pesanan/order-1/struk", "", h.CetakStruk)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("receipt escaping/privacy", w.Body.String())
	}
}

type fakeTracking func(context.Context, string) (service.TrackingView, error)

func (f fakeTracking) Get(ctx context.Context, s string) (service.TrackingView, error) {
	return f(ctx, s)
}
func TestTrackingResponseContainsOnlyPublicProjection(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	h := New(Dependencies{Tracking: fakeTracking(func(_ context.Context, token string) (service.TrackingView, error) {
		if token != base64.RawURLEncoding.EncodeToString(make([]byte, 32)) {
			t.Fatal("token missing")
		}
		return service.TrackingView{Status: domain.Processing, EstimatedReadyAt: time.Date(2026, 11, 8, 8, 0, 0, 0, time.UTC)}, nil
	})})
	r := httptest.NewRequest("GET", "/api/tracking/"+token, nil)
	r.SetPathValue("token", token)
	w := httptest.NewRecorder()
	h.Tracking(w, r)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body) != 4 || strings.Contains(w.Body.String(), token) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unsafe public response", w.Body.String())
	}
}
