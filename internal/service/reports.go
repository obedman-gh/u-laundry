package service

import (
	"context"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type ReportMeta struct {
	ID     string
	Window repository.ReportWindow
}
type DashboardReport struct {
	ReportMeta
	Metrics repository.DashboardMetrics
}
type OrderReport struct {
	ReportMeta
	Filter repository.ReportOrderFilter
	Rows   repository.ReportPage[repository.ReportOrder]
}
type LedgerReport struct {
	ReportMeta
	Page repository.Page
	Rows repository.ReportPage[domain.LedgerEntry]
}

func reportWindow(period ReportPeriod, at time.Time) (repository.ReportWindow, error) {
	from, to := period.From, period.To
	if from.IsZero() && to.IsZero() {
		local := at.In(time.FixedZone("WITA", 8*60*60))
		from = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		to = from.AddDate(0, 0, 1)
	}
	from, to = from.UTC().Truncate(time.Microsecond), to.UTC().Truncate(time.Microsecond)
	if from.IsZero() || to.IsZero() || from.Year() < 1 || to.Year() > 9999 || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return repository.ReportWindow{}, invalid("period", "Isi periode berurutan maksimal 366 hari dengan zona waktu.")
	}
	return repository.ReportWindow{From: from, To: to, AsOf: at}, nil
}

// All report queries share the View's repeatable-read snapshot. Audit is then
// committed separately with a fresh authorization check before returning data.
// Either read/commit/audit failure suppresses the entire response.
func (s *Reports) read(ctx context.Context, actor string, period ReportPeriod, fn func(repository.Readers, repository.ReportWindow) error) (ReportMeta, error) {
	var meta ReportMeta
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, GenerateReport); err != nil {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		window, err := reportWindow(period, at)
		if err != nil {
			return err
		}
		meta.Window = window
		return fn(r, window)
	})
	if err != nil {
		return ReportMeta{}, err
	}
	id, err := s.runtime.id()
	if err != nil {
		return ReportMeta{}, err
	}
	err = s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, GenerateReport)
		if err != nil {
			return err
		}
		return audit(ctx, tx, s.runtime, a.ID, domain.ReportGenerated, domain.ReportEntity, id, meta.Window.AsOf)
	})
	if err != nil {
		return ReportMeta{}, err
	}
	meta.ID = id
	return meta, nil
}

func (s *Reports) Dashboard(ctx context.Context, actor string, period ReportPeriod) (DashboardReport, error) {
	var v DashboardReport
	meta, err := s.read(ctx, actor, period, func(r repository.Readers, w repository.ReportWindow) error {
		var err error
		v.Metrics, err = r.DashboardMetrics(ctx, w)
		return err
	})
	if err != nil {
		return DashboardReport{}, err
	}
	v.ReportMeta = meta
	return v, nil
}

func (s *Reports) OrderDetails(ctx context.Context, actor string, period ReportPeriod, filter repository.ReportOrderFilter) (OrderReport, error) {
	var v OrderReport
	meta, err := s.read(ctx, actor, period, func(r repository.Readers, w repository.ReportWindow) error {
		if filter.Kind == "" {
			filter.Kind = "created"
		}
		if (filter.Kind != "created" && filter.Kind != "receivable" && filter.Kind != "overdue") || (filter.Status != "" && !filter.Status.Valid()) {
			return invalid("filter", "Filter laporan pesanan tidak valid.")
		}
		p, err := filter.Page.Normalize()
		if err != nil {
			return err
		}
		filter.Page, filter.Window = p, w
		v.Filter = filter
		v.Rows, err = r.ReportOrders(ctx, filter)
		return err
	})
	if err != nil {
		return OrderReport{}, err
	}
	v.ReportMeta = meta
	return v, nil
}

func (s *Reports) LedgerDetails(ctx context.Context, actor string, period ReportPeriod, page repository.Page) (LedgerReport, error) {
	var v LedgerReport
	meta, err := s.read(ctx, actor, period, func(r repository.Readers, w repository.ReportWindow) error {
		p, err := page.Normalize()
		if err != nil {
			return err
		}
		v.Page = p
		v.Rows, err = r.ReportLedger(ctx, w, p)
		return err
	})
	if err != nil {
		return LedgerReport{}, err
	}
	v.ReportMeta = meta
	return v, nil
}
