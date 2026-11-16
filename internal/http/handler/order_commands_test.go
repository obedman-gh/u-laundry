package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type commandOrders struct {
	fakeOrders
	change  func(context.Context, string, string, domain.OrderStatus, int64, string) (repository.OrderRecord, error)
	cancel  func(context.Context, string, string, int64, string, string) (repository.OrderRecord, error)
	history func(context.Context, string, string, repository.Page) ([]domain.OrderStatusEvent, error)
}

func (f commandOrders) ChangeStatus(ctx context.Context, actor, id string, to domain.OrderStatus, version int64, key string) (repository.OrderRecord, error) {
	return f.change(ctx, actor, id, to, version, key)
}
func (f commandOrders) Cancel(ctx context.Context, actor, id string, version int64, reason, key string) (repository.OrderRecord, error) {
	return f.cancel(ctx, actor, id, version, reason, key)
}
func (f commandOrders) History(ctx context.Context, actor, id string, page repository.Page) ([]domain.OrderStatusEvent, error) {
	return f.history(ctx, actor, id, page)
}

func TestOrderCommandHTTPKeys(t *testing.T) {
	for _, operation := range []string{"create", "status", "cancel"} {
		for _, tc := range []struct {
			name  string
			keys  []string
			valid bool
		}{
			{"missing", nil, false},
			{"empty", []string{""}, false},
			{"duplicate", []string{"key-1", "key-1"}, false},
			{"invalid", []string{"key/1"}, false},
			{"too_long", []string{strings.Repeat("x", 101)}, false},
			{"valid", []string{"key-1"}, true},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				calls := 0
				o := sampleOrder(t)
				check := func(actor, id, key string) {
					t.Helper()
					calls++
					if actor != "actor-1" || id != o.ID() || key != "key-1" {
						t.Fatal("identity/key not delegated")
					}
				}
				use := commandOrders{
					fakeOrders: fakeOrders{create: func(_ context.Context, actor string, in service.OrderInput) (service.CreatedOrder, error) {
						check(actor, o.ID(), in.IdempotencyKey)
						return service.CreatedOrder{Order: o, Version: 1}, nil
					}},
					change: func(_ context.Context, actor, id string, to domain.OrderStatus, version int64, key string) (repository.OrderRecord, error) {
						check(actor, id, key)
						if to != domain.Processing || version != 1 {
							t.Fatal("status payload lost")
						}
						return repository.OrderRecord{Order: o, Version: 2}, nil
					},
					cancel: func(_ context.Context, actor, id string, version int64, reason, key string) (repository.OrderRecord, error) {
						check(actor, id, key)
						if version != 1 || reason != "Permintaan pelanggan" {
							t.Fatal("cancel payload lost")
						}
						return repository.OrderRecord{Order: o, Version: 2}, nil
					},
				}
				h := New(Dependencies{Orders: use})
				method, path, body, endpoint := "POST", "/api/pesanan", `{"customer_id":"c-1","items":[{"service_id":"s-1","grams":2500}]}`, http.HandlerFunc(h.BikinPesananBaru)
				if operation == "status" {
					method, path, body, endpoint = "PUT", "/api/pesanan/order-1", `{"status":"processing","version":1}`, h.UpdateStatusPesanan
				}
				if operation == "cancel" {
					path, body, endpoint = "/api/pesanan/order-1/cancel", `{"version":1,"reason":"Permintaan pelanggan"}`, h.CancelOrder
				}
				w := protected(t, method, path, body, func(w http.ResponseWriter, r *http.Request) {
					r.Header.Del("Idempotency-Key")
					for _, key := range tc.keys {
						r.Header.Add("Idempotency-Key", key)
					}
					endpoint(w, r)
				})
				if tc.valid {
					want := 200
					if operation == "create" {
						want = 201
					}
					if w.Code != want || calls != 1 {
						t.Fatal(w.Code, w.Body.String(), calls)
					}
				} else {
					checkError(t, w, 400, "validation_failed")
					if calls != 0 {
						t.Fatal("invalid header reached use case")
					}
				}
			})
		}
	}
}

func TestOrderCreateHTTPReplay(t *testing.T) {
	o := sampleOrder(t)
	h := New(Dependencies{Orders: fakeOrders{create: func(context.Context, string, service.OrderInput) (service.CreatedOrder, error) {
		return service.CreatedOrder{Order: o, Version: 4, Replayed: true}, nil
	}}})
	w := protected(t, "POST", "/api/pesanan", `{"customer_id":"c-1","items":[{"service_id":"s-1","grams":2500}]}`, h.BikinPesananBaru)
	var body dto.CreatedOrderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || !body.Replayed || !body.Order.Replayed || body.Order.Version != 4 || body.Order.AppliedVersion != 1 || w.Header().Get("Location") != "/api/pesanan/order-1" {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "tracking_token") || strings.Contains(w.Body.String(), "tracking_expires_at") {
		t.Fatal("capability returned on replay")
	}
}

func TestOrderHistoryHTTP(t *testing.T) {
	calls := 0
	h := New(Dependencies{Orders: commandOrders{history: func(_ context.Context, actor, id string, page repository.Page) ([]domain.OrderStatusEvent, error) {
		calls++
		if actor != "actor-1" || id != "order-1" || page.Limit != 2 || page.Offset != 1 {
			t.Fatal("history identity/pagination lost")
		}
		return []domain.OrderStatusEvent{{Version: 3, From: domain.Processing, To: domain.Ready, ActorID: actor, Source: "staff"}}, nil
	}}})
	w := protected(t, "GET", "/api/pesanan/order-1/history?limit=2&offset=1", "", h.OrderHistory)
	var body []dto.OrderStatusEventResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body) != 1 || body[0].Version != 3 || body[0].From != "processing" || body[0].To != "ready" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, query := range []string{"limit=101", "offset=-1", "limit=1&limit=2", "token=secret"} {
		w = protected(t, "GET", "/api/pesanan/order-1/history?"+query, "", h.OrderHistory)
		checkError(t, w, 400, "validation_failed")
	}
	if calls != 1 {
		t.Fatal("invalid history query delegated")
	}
}
