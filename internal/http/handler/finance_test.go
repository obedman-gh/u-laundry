package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type financeStub struct {
	PaymentUseCase
	refund  func(context.Context, string, service.RefundInput) (repository.RefundResult, error)
	list    func(context.Context, string, string, repository.Page) ([]domain.Payment, error)
	refunds func(context.Context, string, string, repository.Page) ([]domain.Refund, error)
}

func (f financeStub) Refund(c context.Context, a string, in service.RefundInput) (repository.RefundResult, error) {
	return f.refund(c, a, in)
}
func (f financeStub) List(c context.Context, a, id string, p repository.Page) ([]domain.Payment, error) {
	return f.list(c, a, id, p)
}
func (f financeStub) Refunds(c context.Context, a, id string, p repository.Page) ([]domain.Refund, error) {
	return f.refunds(c, a, id, p)
}

func TestRefundHTTPContract(t *testing.T) {
	for _, replay := range []bool{false, true} {
		calls := 0
		h := New(Dependencies{Payments: financeStub{refund: func(_ context.Context, a string, in service.RefundInput) (repository.RefundResult, error) {
			calls++
			if a != "actor-1" || in.OrderID != "order-1" || in.PaymentID != "payment-1" || in.IdempotencyKey != "command-test" || in.Reason != "Koreksi pembayaran" {
				t.Fatal("refund identity/input lost")
			}
			return repository.RefundResult{Refund: domain.Refund{ID: "refund-1", OrderID: in.OrderID, PaymentID: in.PaymentID, IdempotencyKey: in.IdempotencyKey, RecordedBy: a, Amount: 10000, Method: domain.QRIS, RefundedAt: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), Reason: in.Reason}, Replayed: replay}, nil
		}}})
		w := protected(t, "POST", "/api/pesanan/order-1/refunds", `{"payment_id":"payment-1","reason":"Koreksi pembayaran"}`, h.RefundPayment)
		var result dto.RefundResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		want := 201
		if replay {
			want = 200
		}
		if w.Code != want || calls != 1 || result.Method != "qris" || result.Status != "posted" || result.Amount != 10000 || result.Replayed != replay || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, body := range []string{`{"payment_id":"payment-1","reason":""}`, `{"payment_id":"payment-1","reason":"Koreksi","amount":1}`, `{"payment_id":"payment-1","reason":"Koreksi","method":"cash"}`, `{"reason":"Koreksi"}`} {
			w = protected(t, "POST", "/api/pesanan/order-1/refunds", body, h.RefundPayment)
			if w.Code != 400 || calls != 1 {
				t.Fatal("invalid refund reached use case", w.Code, w.Body.String())
			}
		}
		for _, keys := range [][]string{nil, {"bad/key"}, {"a", "b"}} {
			w = protected(t, "POST", "/api/pesanan/order-1/refunds", `{"payment_id":"payment-1","reason":"Koreksi pembayaran"}`, func(w http.ResponseWriter, r *http.Request) {
				r.Header.Del("Idempotency-Key")
				for _, key := range keys {
					r.Header.Add("Idempotency-Key", key)
				}
				h.RefundPayment(w, r)
			})
			if w.Code != 400 || calls != 1 {
				t.Fatal("invalid refund key delegated")
			}
		}
	}
}

func TestFinanceHistoryHTTP(t *testing.T) {
	h := New(Dependencies{Payments: financeStub{
		list: func(_ context.Context, a, id string, p repository.Page) ([]domain.Payment, error) {
			if a != "actor-1" || id != "order-1" || p.Limit != 2 || p.Offset != 1 {
				t.Fatal("payment query lost")
			}
			return []domain.Payment{{ID: "p1", OrderID: id, IdempotencyKey: "paid", Amount: 10000, Method: domain.Cash, Status: domain.PaymentRefunded, RecordedBy: a, ReceivedAt: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)}}, nil
		},
		refunds: func(context.Context, string, string, repository.Page) ([]domain.Refund, error) { return nil, nil },
	}})
	w := protected(t, "GET", "/api/pesanan/order-1/pembayaran?limit=2&offset=1", "", h.ListPayments)
	var rows []dto.PaymentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(rows) != 1 || rows[0].Status != "refunded" || rows[0].CashierID != "actor-1" || rows[0].IdempotencyKey != "paid" || rows[0].ReceivedAt.IsZero() {
		t.Fatal(w.Code, w.Body.String())
	}
	w = protected(t, "GET", "/api/pesanan/order-1/refunds", "", h.ListRefunds)
	if w.Code != 200 || w.Body.String() != "[]\n" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, endpoint := range []http.HandlerFunc{h.ListPayments, h.ListRefunds} {
		w = protected(t, "GET", "/api/pesanan/order-1/refunds?limit=101", "", endpoint)
		checkError(t, w, 400, "validation_failed")
	}
}
