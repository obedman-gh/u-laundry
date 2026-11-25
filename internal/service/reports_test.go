package service

import (
	"context"
	"errors"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type reportTestWork struct {
	repository.UnitOfWork
	window    repository.ReportWindow
	err       error
	afterRead func()
}
type reportTestReader struct {
	repository.Readers
	work *reportTestWork
}

func (w *reportTestWork) View(ctx context.Context, fn func(repository.Readers) error) error {
	err := w.UnitOfWork.View(ctx, func(r repository.Readers) error { return fn(reportTestReader{r, w}) })
	if err == nil && w.afterRead != nil {
		w.afterRead()
	}
	return err
}
func (r reportTestReader) DashboardMetrics(_ context.Context, w repository.ReportWindow) (repository.DashboardMetrics, error) {
	r.work.window = w
	return repository.DashboardMetrics{Orders: repository.OrderMetrics{Created: 7}}, r.work.err
}

func TestDashboardReportAuthorizationAndFailure(t *testing.T) {
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
		t.Run(string(role), func(t *testing.T) {
			u, rt := fixture(t, role)
			w := &reportTestWork{UnitOfWork: u}
			v, err := NewReports(w, rt).Dashboard(context.Background(), "actor-1", ReportPeriod{})
			if role == domain.Cashier {
				requireDomain(t, err, domain.Forbidden, "")
				if v.ID != "" || !w.window.AsOf.IsZero() {
					t.Fatal("denied report leaked")
				}
				return
			}
			if err != nil || v.Metrics.Orders.Created != 7 || len(u.state.audits) != 1 || u.state.audits[0].Action != domain.ReportGenerated {
				t.Fatal(v, err)
			}
			if w.window.From.Hour() != 16 || w.window.To.Sub(w.window.From) != 24*time.Hour || !w.window.AsOf.Equal(fixed) {
				t.Fatal("WITA boundary", w.window)
			}
		})
	}
	for _, stage := range []string{"read", "audit", "commit", "revoked"} {
		t.Run(stage, func(t *testing.T) {
			u, rt := fixture(t, domain.Owner)
			w := &reportTestWork{UnitOfWork: u}
			switch stage {
			case "read":
				w.err = failure
			case "audit":
				u.failAt = "audit"
			case "commit":
				u.commitErr = failure
			case "revoked":
				w.afterRead = func() { a := u.state.staff["actor-1"]; a.Active = false; u.state.staff[a.ID] = a }
			}
			v, err := NewReports(w, rt).Dashboard(context.Background(), "actor-1", ReportPeriod{})
			if err == nil || v.ID != "" || v.Metrics.Orders.Created != 0 || len(u.state.audits) != 0 {
				t.Fatal("failed report leaked", v, err)
			}
			if stage != "revoked" && !errors.Is(err, failure) {
				t.Fatal(err)
			}
		})
	}
}
func TestReportWindowValidation(t *testing.T) {
	for _, p := range []ReportPeriod{{From: fixed}, {To: fixed}, {From: fixed, To: fixed}, {From: fixed, To: fixed.Add(time.Nanosecond)}, {From: fixed, To: fixed.Add(367 * 24 * time.Hour)}} {
		if _, err := reportWindow(p, fixed); err == nil {
			t.Fatal("invalid period accepted", p)
		}
	}
	for _, filter := range []repository.ReportOrderFilter{{Kind: "all"}, {Kind: "created", Status: "unknown"}, {Kind: "receivable", Page: repository.Page{Limit: 101}}} {
		u, rt := fixture(t, domain.Admin)
		_, err := NewReports(u, rt).OrderDetails(context.Background(), "actor-1", ReportPeriod{}, filter)
		requireDomain(t, err, domain.Validation, "")
		if len(u.state.audits) != 0 {
			t.Fatal("invalid filter audited as success")
		}
	}
}
