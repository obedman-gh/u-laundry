package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ulaundry/internal/repository"
	"ulaundry/internal/service"
	"ulaundry/internal/telemetry"
)

type healthCheck func(context.Context) error

func (f healthCheck) Check(ctx context.Context) error { return f(ctx) }
func TestLivenessReadinessStatesAndTimeout(t *testing.T) {
	for _, name := range []string{"ready", "unavailable", "missing", "timeout", "draining", "drain-during-ping"} {
		t.Run(name, func(t *testing.T) {
			calls, draining := 0, name == "draining"
			use := Dependencies{ReadinessTimeout: 20 * time.Millisecond, Draining: func() bool { return draining }, Health: healthCheck(func(ctx context.Context) error {
				calls++
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 25*time.Millisecond {
					t.Fatal("dependency lacks bounded deadline")
				}
				switch name {
				case "unavailable":
					return errors.New("private-db-dsn")
				case "timeout":
					<-ctx.Done()
					return ctx.Err()
				case "drain-during-ping":
					draining = true
				}
				return nil
			})}
			if name == "missing" {
				use.Health = nil
			}
			h := New(use)
			for _, method := range []string{"GET", "HEAD"} {
				w := httptest.NewRecorder()
				h.Liveness(w, httptest.NewRequest(method, "/api/health/live", nil))
				if w.Code != 200 || calls != 0 || method == "HEAD" && w.Body.Len() != 0 {
					t.Fatal("liveness touched database")
				}
			}
			w := httptest.NewRecorder()
			h.Readiness(w, httptest.NewRequest("GET", "/api/health/ready", nil))
			want := 503
			if name == "ready" {
				want = 200
			}
			if w.Code != want || strings.Contains(w.Body.String(), "private-") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("readiness", w.Code, w.Body.String())
			}
			if (name == "missing" || name == "draining") && calls != 0 {
				t.Fatal("unneeded dependency query")
			}
		})
	}
}
func TestBusinessMetricsExcludeReplaysAndFailedUseCases(t *testing.T) {
	for _, operation := range []string{"order", "payment", "refund"} {
		t.Run(operation, func(t *testing.T) {
			m := telemetry.NewRegistry()
			attempt := 0
			result := func() (bool, error) {
				attempt++
				if attempt == 3 {
					return false, repository.ErrUnavailable
				}
				return attempt == 2, nil
			}
			o := sampleOrder(t)
			h := New(Dependencies{Metrics: m, Orders: fakeOrders{create: func(context.Context, string, service.OrderInput) (service.CreatedOrder, error) {
				replay, err := result()
				return service.CreatedOrder{Order: o, Version: 1, Replayed: replay}, err
			}}, Payments: financeStub{PaymentUseCase: fakePayment(func(context.Context, string, service.PaymentInput) (repository.PaymentResult, error) {
				replay, err := result()
				return repository.PaymentResult{Replayed: replay}, err
			}), refund: func(context.Context, string, service.RefundInput) (repository.RefundResult, error) {
				replay, err := result()
				return repository.RefundResult{Replayed: replay}, err
			}}})
			fn, body := http.HandlerFunc(h.BikinPesananBaru), `{"customer_id":"c-1","items":[{"service_id":"s-1","grams":2500}]}`
			if operation == "payment" {
				fn, body = h.RecordPayment, `{"amount":1000,"method":"cash"}`
			}
			if operation == "refund" {
				fn, body = h.RefundPayment, `{"payment_id":"payment-1","reason":"Koreksi pembayaran"}`
			}
			for _, want := range []int{201, 200, 503} {
				w := protected(t, "POST", "/api/pesanan/order-1", body, fn)
				if w.Code != want {
					t.Fatal("unexpected response", w.Code, w.Body.String())
				}
			}
			b := m.Snapshot().Business
			if b.OrdersCreated+b.PaymentsRecorded+b.RefundsRecorded != 1 {
				t.Fatal("replay/failure counted", b)
			}
			switch operation {
			case "order":
				if b.OrdersCreated != 1 {
					t.Fatal(b)
				}
			case "payment":
				if b.PaymentsRecorded != 1 {
					t.Fatal(b)
				}
			case "refund":
				if b.RefundsRecorded != 1 {
					t.Fatal(b)
				}
			}
		})
	}
}
