package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
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
		`{"name":"X","unit":"item","turnaround_minutes":60,"active":true}`,
		`{"name":"X","unit":"item","unit_price":null,"turnaround_minutes":60,"active":true}`,
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
func TestCatalogHTTPExplicitZeroPrice(t *testing.T) {
	calls := 0
	h := New(Dependencies{Catalog: catalogStub{configure: func(_ context.Context, _ string, in service.ServiceInput) (repository.ServiceRecord, error) {
		calls++
		if in.UnitPrice != 0 || in.Active {
			t.Fatal("explicit free inactive service changed")
		}
		return repository.ServiceRecord{Service: domain.Service{ID: "free", Name: in.Name, Unit: in.Unit, UnitPrice: in.UnitPrice, Turnaround: in.Turnaround}, Version: 1}, nil
	}}})
	w := protected(t, "POST", "/api/services", `{"name":"Contoh Gratis","unit":"kg","unit_price":0,"turnaround_minutes":60,"active":false}`, h.ConfigureService)
	if w.Code != 201 || calls != 1 || !strings.Contains(w.Body.String(), `"unit_price":0`) {
		t.Fatal("zero price rejected", w.Code, w.Body.String())
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
	var body dto.OrderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body.Subtotal == nil || *body.Subtotal != 17500 || body.Discount == nil || *body.Discount != 0 || body.Total != 17500 || len(body.Items) != 1 {
		t.Fatal("snapshot response", w.Body.String())
	}
	item := body.Items[0]
	if item.ServiceID != "s-1" || item.Name != "Cuci" || item.Unit != "kg" || item.UnitPrice != 7000 || item.Grams != 2500 || item.Pieces != 0 || item.LineTotal != 17500 {
		t.Fatal("incomplete item snapshot", w.Body.String())
	}
}
