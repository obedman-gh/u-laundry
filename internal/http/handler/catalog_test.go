package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type catalogStub struct {
	CatalogUseCase
	configure func(context.Context, string, service.ServiceInput) (repository.ServiceRecord, error)
	list      func(context.Context, string, repository.ServiceFilter) ([]repository.ServiceRecord, error)
	get       func(context.Context, string, string) (repository.ServiceRecord, error)
	history   func(context.Context, string, string, repository.Page) ([]repository.ServiceRevision, error)
}

func (s catalogStub) Configure(c context.Context, a string, in service.ServiceInput) (repository.ServiceRecord, error) {
	return s.configure(c, a, in)
}
func (s catalogStub) List(c context.Context, a string, f repository.ServiceFilter) ([]repository.ServiceRecord, error) {
	return s.list(c, a, f)
}
func (s catalogStub) Get(c context.Context, a, id string) (repository.ServiceRecord, error) {
	return s.get(c, a, id)
}
func (s catalogStub) History(c context.Context, a, id string, p repository.Page) ([]repository.ServiceRevision, error) {
	return s.history(c, a, id, p)
}

func TestCatalogHTTPConfigureAndUnitCompatibility(t *testing.T) {
	calls := 0
	h := New(Dependencies{Catalog: catalogStub{configure: func(_ context.Context, a string, in service.ServiceInput) (repository.ServiceRecord, error) {
		calls++
		if a != "actor-1" || in.Unit != domain.PerPiece || in.DisplayOrder != 5 || in.UnitPrice != 20000 || in.Turnaround != 48*time.Hour || !in.Active {
			t.Fatal("catalog translation")
		}
		return repository.ServiceRecord{Service: domain.Service{ID: "service-1", Name: in.Name, Unit: in.Unit, UnitPrice: in.UnitPrice, Turnaround: in.Turnaround, Active: in.Active, DisplayOrder: in.DisplayOrder}, Version: 1}, nil
	}}})
	for _, unit := range []string{"item", "piece"} {
		w := protected(t, "POST", "/api/services", `{"name":"Selimut","unit":"`+unit+`","unit_price":20000,"turnaround_minutes":2880,"active":true,"display_order":5}`, h.ConfigureService)
		if w.Code != 201 || !strings.Contains(w.Body.String(), `"unit":"item"`) || !strings.Contains(w.Body.String(), `"display_order":5`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, body := range []string{
		`{"name":"X","unit":"item","unit_price":-1,"turnaround_minutes":60,"active":true}`,
		`{"name":"X","unit":"item","unit_price":1.5,"turnaround_minutes":60,"active":true}`,
		`{"name":"X","unit":"litre","unit_price":1,"turnaround_minutes":60,"active":true}`,
		`{"name":"X","unit":"item","unit_price":1,"turnaround_minutes":0,"active":true}`,
		`{"name":"X","unit":"item","unit_price":1,"turnaround_minutes":153722868,"active":true}`,
		`{"name":"X","unit":"item","unit_price":1,"turnaround_minutes":60,"active":true,"display_order":-1}`,
		`{"name":"X","unit":"item","unit_price":1,"turnaround_minutes":60,"active":true,"display_order":1000001}`,
		`{"name":"X","unit":"item","unit_price":1,"turnaround_minutes":60,"active":true,"actor_id":"owner"}`,
		`{"name":"X","unit":"item","unit_price":1,"turnaround_minutes":60,"active":true,"version":1}`,
	} {
		w := protected(t, "POST", "/api/services", body, h.ConfigureService)
		if w.Code != 400 || calls != 2 {
			t.Fatal("invalid config delegated", w.Code, w.Body.String())
		}
	}
	w := protected(t, "PUT", "/api/services/order-1", `{"name":"X","unit":"item","unit_price":1,"turnaround_minutes":60,"active":true}`, h.ConfigureService)
	if w.Code != 400 || calls != 2 {
		t.Fatal("update missing version")
	}
}
func TestCatalogHTTPListDetailAndHistory(t *testing.T) {
	r := repository.ServiceRecord{Service: domain.Service{ID: "order-1", Name: "Synthetic", Unit: domain.PerPiece, UnitPrice: 1000, Turnaround: time.Hour, DisplayOrder: 3}, Version: 2}
	calls := 0
	h := New(Dependencies{Catalog: catalogStub{
		list: func(_ context.Context, a string, f repository.ServiceFilter) ([]repository.ServiceRecord, error) {
			calls++
			if a != "actor-1" || !f.IncludeInactive || f.Page.Limit != 10 || f.Page.Offset != 20 {
				t.Fatal("catalog list filter")
			}
			return []repository.ServiceRecord{r}, nil
		},
		get: func(_ context.Context, a, id string) (repository.ServiceRecord, error) {
			if a != "actor-1" || id != "order-1" {
				t.Fatal("catalog detail identity")
			}
			return r, nil
		},
		history: func(_ context.Context, a, id string, p repository.Page) ([]repository.ServiceRevision, error) {
			if a != "actor-1" || id != "order-1" || p.Limit != 10 {
				t.Fatal("history scope")
			}
			return []repository.ServiceRevision{{ServiceRecord: r, ActorID: "actor-1", Source: "staff"}}, nil
		},
	}})
	w := protected(t, "GET", "/api/services?include_inactive=true&limit=10&offset=20", "", h.ListServices)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"active":false`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, query := range []string{"?include_inactive=1", "?include_inactive=", "?include_inactive=true&include_inactive=false", "?limit=101", "?price=1"} {
		w = protected(t, "GET", "/api/services"+query, "", h.ListServices)
		if w.Code != 400 || calls != 1 {
			t.Fatal("invalid list query")
		}
	}
	w = protected(t, "GET", "/api/services/order-1", "", h.GetService)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":2`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = protected(t, "GET", "/api/services/order-1/history?limit=10", "", h.ServiceHistory)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"source":"staff"`) || !strings.Contains(w.Body.String(), `"changed_by":"actor-1"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestCatalogOrderSnapshotResponse(t *testing.T) {
	o := sampleOrder(t)
	h := New(Dependencies{Orders: fakeOrders{get: func(context.Context, string, string) (repository.OrderRecord, error) {
		return repository.OrderRecord{Order: o, Version: 1}, nil
	}}})
	w := protected(t, "GET", "/api/pesanan/order-1", "", h.GetPesananById)
	var body struct {
		Subtotal, Discount, Total int64
		Items                     []struct {
			Unit                        string
			UnitPrice, Grams, LineTotal int64
		}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"items", "subtotal", "discount", "total_biaya"} {
		if _, ok := raw[key]; !ok {
			t.Fatal("missing financial snapshot", key)
		}
	}
	if err := json.Unmarshal(raw["subtotal"], &body.Subtotal); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw["discount"], &body.Discount); err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(raw["items"], &items); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body.Subtotal != 17500 || body.Discount != 0 || len(items) != 1 || items[0]["unit_price"] != float64(7000) || items[0]["grams"] != float64(2500) || items[0]["line_total"] != float64(17500) {
		t.Fatal("snapshot response", w.Body.String())
	}
}
