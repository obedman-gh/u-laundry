package service

import (
	"context"
	"errors"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type Tracking struct{ uow repository.UnitOfWork }

func NewTracking(uow repository.UnitOfWork) *Tracking { return &Tracking{uow} }

// TrackingView is an explicit privacy projection: no name, contact, order ID,
// notes, services, price, payment details, actor, or tracking token.
type TrackingView struct {
	Status               domain.OrderStatus
	EstimatedReadyAt     time.Time
	ReadyAt, CollectedAt time.Time
}

func (s *Tracking) Get(ctx context.Context, token string) (TrackingView, error) {
	missing := &domain.Error{Kind: domain.NotFound, Code: "tracking_not_found", Field: "tracking", Message: "Status pesanan tidak ditemukan."}
	hash, ok := tokenHash(token)
	if !ok {
		return TrackingView{}, missing
	}
	var result TrackingView
	err := s.uow.View(ctx, func(r repository.Readers) error {
		v, err := r.GetTracking(ctx, hash)
		if err != nil {
			return err
		}
		result = TrackingView{v.Status, v.EstimatedReadyAt, v.ReadyAt, v.CollectedAt}
		return nil
	})
	if errors.Is(err, repository.ErrNotFound) {
		return TrackingView{}, missing
	}
	if err != nil {
		return TrackingView{}, err
	}
	return result, nil
}

type Reports struct {
	uow     repository.UnitOfWork
	runtime Runtime
}

func NewReports(uow repository.UnitOfWork, rt Runtime) *Reports { return &Reports{uow, rt} }

type ReportPeriod struct{ From, To time.Time }
type ReportData struct {
	ID                       string
	From, To, GeneratedAt    time.Time
	Inflow, Outflow, Balance domain.Money
}

func (s *Reports) Generate(ctx context.Context, actor string, period ReportPeriod) (ReportData, error) {
	var result ReportData
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, GenerateReport)
		if err != nil {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		from, to := period.From, period.To
		if from.IsZero() && to.IsZero() {
			local := at.In(time.FixedZone("WITA", 8*60*60))
			from = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
			to = from.Add(24 * time.Hour)
		}
		from, to = from.UTC().Truncate(time.Microsecond), to.UTC().Truncate(time.Microsecond)
		if from.IsZero() || to.IsZero() || !to.After(from) {
			return invalid("period", "Periode laporan harus mempunyai awal dan akhir yang berurutan.")
		}
		totals, err := tx.LedgerTotals(ctx, from, to)
		if err != nil {
			return err
		}
		if totals.Inflow < 0 || totals.Outflow < 0 {
			return &repository.Error{Kind: repository.InvalidData, Operation: "generate report"}
		}
		id, err := s.runtime.id()
		if err != nil {
			return err
		}
		if err := audit(ctx, tx, s.runtime, a.ID, domain.ReportGenerated, domain.ReportEntity, id, at); err != nil {
			return err
		}
		result = ReportData{ID: id, From: from, To: to, GeneratedAt: at, Inflow: totals.Inflow, Outflow: totals.Outflow, Balance: totals.Inflow - totals.Outflow}
		return nil
	})
	if err != nil {
		return ReportData{}, err
	}
	return result, nil
}
