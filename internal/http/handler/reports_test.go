package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type reportStub struct {
	ReportUseCase
	dashboard func(context.Context, string, service.ReportPeriod) (service.DashboardReport, error)
	orders    func(context.Context, string, service.ReportPeriod, repository.ReportOrderFilter) (service.OrderReport, error)
	ledger    func(context.Context, string, service.ReportPeriod, repository.Page) (service.LedgerReport, error)
}

func (s reportStub) Dashboard(c context.Context, a string, p service.ReportPeriod) (service.DashboardReport, error) {
	return s.dashboard(c, a, p)
}
func (s reportStub) OrderDetails(c context.Context, a string, p service.ReportPeriod, f repository.ReportOrderFilter) (service.OrderReport, error) {
	return s.orders(c, a, p, f)
}
func (s reportStub) LedgerDetails(c context.Context, a string, p service.ReportPeriod, f repository.Page) (service.LedgerReport, error) {
	return s.ledger(c, a, p, f)
}
func TestReportHTTPBoundary(t *testing.T) {
	calls := 0
	h := New(Dependencies{Reports: reportStub{dashboard: func(_ context.Context, a string, p service.ReportPeriod) (service.DashboardReport, error) {
		calls++
		if a != "actor-1" {
			t.Fatal(a)
		}
		return service.DashboardReport{}, nil
	}}})
	for _, q := range []string{"from=bad&to=bad", "from=2026-11-25", "from=2026-11-25T00:00:00Z", "from=2026-11-25T00:00:00Z&to=2026-11-25T00:00:00Z", "from=x&from=y", "actor=other", "from=2025-01-01T00:00:00Z&to=2026-11-25T00:00:00Z"} {
		w := protected(t, "GET", "/api/laporan/dashboard?"+q, "", h.DashboardReport)
		if w.Code != 400 || calls != 0 {
			t.Fatal("invalid query delegated", q, w.Code)
		}
	}
	w := protected(t, "GET", "/api/laporan/dashboard", "", h.DashboardReport)
	var v dto.DashboardResponse
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || v.Methods == nil || v.Popular == nil || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, q := range []string{"format=html", "limit=101", "offset=-1", "kind=other", "status=Unknown", "limit=1&limit=2"} {
		w := protected(t, "GET", "/api/laporan/orders?"+q, "", h.ReportOrders)
		if w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
	h = New(Dependencies{Reports: reportStub{ledger: func(context.Context, string, service.ReportPeriod, repository.Page) (service.LedgerReport, error) {
		return service.LedgerReport{}, &service.AccessDenied{}
	}}})
	w = protected(t, "GET", "/api/laporan/ledger?format=csv", "", h.ReportLedger)
	if w.Code != 403 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Content-Disposition") != "" {
		t.Fatal("denied CSV", w.Code, w.Body.String())
	}
}
func TestReportCSVSafeAndHead(t *testing.T) {
	at := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	meta := service.ReportMeta{ID: "report-test", Window: repository.ReportWindow{From: at, To: at.Add(24 * time.Hour), AsOf: at}}
	h := New(Dependencies{Reports: reportStub{
		orders: func(_ context.Context, a string, _ service.ReportPeriod, f repository.ReportOrderFilter) (service.OrderReport, error) {
			if a != "actor-1" || f.Page.Limit != 1 || f.Page.Offset != 2 || f.Kind != "receivable" {
				t.Fatal(a, f)
			}
			return service.OrderReport{ReportMeta: meta, Filter: f, Rows: repository.ReportPage[repository.ReportOrder]{HasMore: true, Items: []repository.ReportOrder{{ID: "order-1", CustomerName: " \t=HYPERLINK(\"bad\")", Total: 10000, Due: 10000, CreatedAt: at, EstimatedReadyAt: at}}}}, nil
		},
		ledger: func(context.Context, string, service.ReportPeriod, repository.Page) (service.LedgerReport, error) {
			return service.LedgerReport{ReportMeta: meta, Page: repository.Page{Limit: 20}, Rows: repository.ReportPage[domain.LedgerEntry]{Items: []domain.LedgerEntry{{ID: "entry-1", Description: "+SUM(1,2)", Amount: 123, OccurredAt: at}}}}, nil
		},
	}})
	w := protected(t, "GET", "/api/laporan/orders?kind=receivable&limit=1&offset=2&format=csv", "", h.ReportOrders)
	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil || w.Code != 200 || len(rows) != 2 || !strings.HasPrefix(rows[1][1], "'") || rows[1][3] != "10000" || !strings.HasSuffix(rows[1][6], "+08:00") || w.Header().Get("X-Report-Has-More") != "true" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(rows, w.Code, err)
	}
	w = protected(t, "HEAD", "/api/laporan/ledger?format=csv", "", h.ReportLedger)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") == "0" {
		t.Fatal("HEAD CSV", w.Code, w.Body.String())
	}
	w = protected(t, "GET", "/api/laporan/ledger?format=csv", "", h.ReportLedger)
	rows, err = csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil || rows[1][8] != "'+SUM(1,2)" || rows[1][7] != "123" {
		t.Fatal(rows, err)
	}
	for _, x := range []string{"=1", " +1", "-2", "@SUM(1)", "\tbad", "\rformula", "\u2003=1", "a\nb"} {
		if !strings.HasPrefix(csvText(x), "'") {
			t.Fatal("unsafe", x)
		}
	}
	for _, x := range []string{"Pelanggan Contoh", "a, b", "\"quoted\"", ""} {
		if csvText(x) != x {
			t.Fatal("changed safe text", x)
		}
	}
}
