package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/service"
)

type shiftStub struct {
	ShiftUseCase
	open    func(context.Context, string, service.ShiftOpenInput) (service.ShiftView, error)
	close   func(context.Context, string, service.ShiftCloseInput) (service.ShiftView, error)
	move    func(context.Context, string, service.CashMoveInput) (service.CashMoveResult, error)
	current func(context.Context, string) (*service.ShiftView, error)
	daily   func(context.Context, string, string) (service.ShiftDay, error)
}

func (s shiftStub) Open(c context.Context, a string, in service.ShiftOpenInput) (service.ShiftView, error) {
	return s.open(c, a, in)
}
func (s shiftStub) Close(c context.Context, a string, in service.ShiftCloseInput) (service.ShiftView, error) {
	return s.close(c, a, in)
}
func (s shiftStub) Move(c context.Context, a string, in service.CashMoveInput) (service.CashMoveResult, error) {
	return s.move(c, a, in)
}
func (s shiftStub) Current(c context.Context, a string) (*service.ShiftView, error) {
	return s.current(c, a)
}
func (s shiftStub) Daily(c context.Context, a, date string) (service.ShiftDay, error) {
	return s.daily(c, a, date)
}

func TestShiftHTTPBoundary(t *testing.T) {
	calls := 0
	h := New(Dependencies{Shifts: shiftStub{
		open: func(_ context.Context, a string, in service.ShiftOpenInput) (service.ShiftView, error) {
			calls++
			if a != "actor-1" || in.Opening != 0 || in.Key != "command-test" {
				t.Fatal("open input lost")
			}
			return service.ShiftView{Shift: domain.CashShift{ID: "order-1"}, Replayed: true}, nil
		},
		close: func(_ context.Context, a string, in service.ShiftCloseInput) (service.ShiftView, error) {
			calls++
			if a != "actor-1" || in.ID != "order-1" || in.Physical != 0 || in.Key != "command-test" {
				t.Fatal("close input lost")
			}
			return service.ShiftView{}, nil
		},
		move: func(_ context.Context, a string, in service.CashMoveInput) (service.CashMoveResult, error) {
			calls++
			if a != "actor-1" || in.ID != "order-1" || in.Direction != domain.Credit || in.Amount != 1 || in.Key != "command-test" {
				t.Fatal("movement input lost")
			}
			return service.CashMoveResult{}, nil
		},
	}})
	for _, tc := range []struct {
		name, path, valid string
		fn                http.HandlerFunc
		bad               []string
		status            int
	}{
		{"open", "/api/shifts", `{"opening":0}`, h.OpenShift, []string{`{}`, `{"opening":-1}`, `{"opening":0,"cashier_id":"other"}`, `{"opening":1.5}`}, 200},
		{"close", "/api/shifts/order-1/close", `{"physical":0}`, h.CloseShift, []string{`{}`, `{"physical":-1}`, `{"physical":0,"system_cash":1}`, `{"physical":null}`}, 200},
		{"cash", "/api/shifts/order-1/cash", `{"amount":1,"direction":"out","notes":"Setor kas"}`, h.MoveShiftCash, []string{`{"amount":0,"direction":"out","notes":"Alasan"}`, `{"amount":1,"direction":"other","notes":"Alasan"}`, `{"amount":1,"direction":"in","notes":""}`}, 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, body := range tc.bad {
				before := calls
				w := protected(t, "POST", tc.path, body, tc.fn)
				if w.Code != 400 || calls != before {
					t.Fatal("invalid body delegated", w.Code, w.Body.String())
				}
			}
			for _, keys := range [][]string{nil, {"bad/key"}, {"one", "two"}} {
				before := calls
				w := protected(t, "POST", tc.path, tc.valid, func(w http.ResponseWriter, r *http.Request) {
					r.Header.Del("Idempotency-Key")
					for _, k := range keys {
						r.Header.Add("Idempotency-Key", k)
					}
					tc.fn(w, r)
				})
				if w.Code != 400 || calls != before {
					t.Fatal("invalid key delegated")
				}
			}
			w := protected(t, "POST", tc.path, tc.valid, tc.fn)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
func TestShiftHTTPReadAndDenied(t *testing.T) {
	h := New(Dependencies{Shifts: shiftStub{
		current: func(context.Context, string) (*service.ShiftView, error) { return nil, nil },
		daily: func(_ context.Context, a, date string) (service.ShiftDay, error) {
			if a != "actor-1" || date != "2026-11-25" {
				t.Fatal("daily query lost")
			}
			return service.ShiftDay{Date: date, Totals: domain.ShiftTotals{UnknownOut: 500}}, nil
		},
		close: func(context.Context, string, service.ShiftCloseInput) (service.ShiftView, error) {
			return service.ShiftView{}, &domain.Error{Kind: domain.Forbidden}
		},
	}})
	w := protected(t, "GET", "/api/shifts/current", "", h.CurrentShift)
	if w.Code != 200 || w.Body.String() != "null\n" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = protected(t, "GET", "/api/shifts/daily?date=2026-11-25", "", h.DailyShifts)
	var report dto.ShiftDayResponse
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || report.Shifts == nil || report.Date != "2026-11-25" || report.Totals.UnknownOut != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = protected(t, "GET", "/api/shifts/daily?cashier_id=other", "", h.DailyShifts)
	checkError(t, w, 400, "validation_failed")
	w = protected(t, "POST", "/api/shifts/order-1/close", `{"physical":0}`, h.CloseShift)
	checkError(t, w, 403, "forbidden")
}
