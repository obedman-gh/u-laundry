package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type customerStub struct {
	CustomerUseCase
	search func(context.Context, string, string, repository.Page) ([]domain.Customer, error)
	get    func(context.Context, string, string) (repository.CustomerRecord, error)
	update func(context.Context, string, string, service.CustomerInput, int64) (repository.CustomerRecord, error)
	create func(context.Context, string, service.CustomerInput) (domain.Customer, error)
}

func (s customerStub) Search(c context.Context, a, q string, p repository.Page) ([]domain.Customer, error) {
	return s.search(c, a, q, p)
}
func (s customerStub) Get(c context.Context, a, id string) (repository.CustomerRecord, error) {
	return s.get(c, a, id)
}
func (s customerStub) Update(c context.Context, a, id string, in service.CustomerInput, v int64) (repository.CustomerRecord, error) {
	return s.update(c, a, id, in, v)
}
func (s customerStub) Create(c context.Context, a string, in service.CustomerInput) (domain.Customer, error) {
	return s.create(c, a, in)
}

func TestCustomerHTTPMinimumDataSearchAndHistory(t *testing.T) {
	c := domain.Customer{ID: "order-1", Name: "Synthetic", Contact: "+6281234567890", Address: "private-address", Notes: "private-notes"}
	calls := 0
	h := New(Dependencies{Customers: customerStub{
		search: func(_ context.Context, a, q string, p repository.Page) ([]domain.Customer, error) {
			calls++
			if a != "actor-1" || q != "Synthetic" || p.Limit != 10 || p.Offset != 20 {
				t.Fatal("search delegation")
			}
			return []domain.Customer{c}, nil
		},
		get: func(_ context.Context, a, id string) (repository.CustomerRecord, error) {
			if a != "actor-1" || id != c.ID {
				t.Fatal("detail identity")
			}
			return repository.CustomerRecord{Customer: c, Version: 3}, nil
		},
	}, Orders: fakeOrders{list: func(_ context.Context, a string, f repository.OrderFilter) ([]service.OrderListItem, error) {
		if a != "actor-1" || f.CustomerID != c.ID || f.Page.Limit != 10 {
			t.Fatal("history scope")
		}
		return []service.OrderListItem{{OrderSummary: repository.OrderSummary{ID: "history-1", CustomerID: c.ID, Status: domain.Received, Total: 1000}, PaymentStatus: domain.Unpaid}}, nil
	}}})
	w := protected(t, "GET", "/api/customers?q=Synthetic&limit=10&offset=20", "", h.SearchCustomers)
	var summaries []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(summaries) != 1 || len(summaries[0]) != 3 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("search discloses optional notes", w.Body.String())
	}
	for _, suffix := range []string{"?q=x&q=y", "?q=", "?contact=x", "?limit=101", "?offset=-1"} {
		w = protected(t, "GET", "/api/customers"+suffix, "", h.SearchCustomers)
		if w.Code != 400 || calls != 1 {
			t.Fatal("invalid query reached service", suffix, w.Code)
		}
	}
	w = protected(t, "GET", "/api/customers/order-1", "", h.GetCustomer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":3`) || !strings.Contains(w.Body.String(), "private-address") {
		t.Fatal("authorized detail", w.Body.String())
	}
	w = protected(t, "GET", "/api/customers/order-1/orders?limit=10", "", h.CustomerOrders)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "history-1") || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "tracking_token") {
		t.Fatal("history privacy", w.Body.String())
	}
}
func TestCustomerHTTPCreateUpdateValidation(t *testing.T) {
	calls := 0
	h := New(Dependencies{Customers: customerStub{
		create: func(_ context.Context, a string, in service.CustomerInput) (domain.Customer, error) {
			calls++
			if a != "actor-1" || in.Name != "Synthetic" || in.Contact != "" || in.Address != "" || in.Notes != "" {
				t.Fatal("name-only create")
			}
			return domain.Customer{ID: "c-1", Name: in.Name}, nil
		},
		update: func(_ context.Context, a, id string, in service.CustomerInput, v int64) (repository.CustomerRecord, error) {
			calls++
			if a != "actor-1" || id != "order-1" || v != 2 || in.Address != "Viola" {
				t.Fatal("update delegation")
			}
			return repository.CustomerRecord{Customer: domain.Customer{ID: id, Name: in.Name, Address: in.Address}, Version: 3}, nil
		},
	}})
	w := protected(t, "POST", "/api/customers", `{"name":"Synthetic"}`, h.CreateCustomer)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = protected(t, "PUT", "/api/customers/order-1", `{"name":"Synthetic","address":"Viola","version":2}`, h.UpdateCustomer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":3`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{`{"name":""}`, `{"name":"Synthetic","version":0}`, `{"name":"Synthetic","version":1,"actor_id":"owner"}`, `{"name":"Synthetic","version":1,"birth_date":"2000-01-01"}`, `{"name":"Synthetic","version":1,"address":"` + strings.Repeat("x", 501) + `"}`} {
		w = protected(t, "PUT", "/api/customers/order-1", body, h.UpdateCustomer)
		if w.Code != 400 || calls != 2 {
			t.Fatal("invalid update reached service", w.Code)
		}
	}
}

type rotationStub struct {
	OrderUseCase
	calls int
}

func (s *rotationStub) RotateTracking(_ context.Context, a, id string, v int64) (service.RotatedTracking, error) {
	s.calls++
	if a != "actor-1" || id != "order-1" || v != 3 {
		return service.RotatedTracking{}, &domain.Error{Kind: domain.Forbidden}
	}
	return service.RotatedTracking{Token: "one-time-token", ExpiresAt: time.Date(2027, 2, 7, 0, 0, 0, 0, time.UTC), Version: 4}, nil
}
func TestTrackingRotationHTTPBoundary(t *testing.T) {
	s := &rotationStub{}
	h := New(Dependencies{Orders: s})
	w := protected(t, "POST", "/api/pesanan/order-1/tracking", `{"version":3}`, h.RotateTracking)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":4`) || !strings.Contains(w.Body.String(), `"tracking_expires_at"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{`{}`, `{"version":0}`, `{"version":3,"token":"chosen"}`} {
		w = protected(t, "POST", "/api/pesanan/order-1/tracking", body, h.RotateTracking)
		if w.Code != 400 || s.calls != 1 {
			t.Fatal("unsafe rotation request")
		}
	}
	// The handler still requires actor authorization even if called below routing.
	r := httptest.NewRequest("POST", "/api/pesanan/order-1/tracking", strings.NewReader(`{"version":3}`))
	r.SetPathValue("id", "order-1")
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.RotateTracking(w, r)
	if w.Code != 403 {
		t.Fatal("actor missing")
	}
}
