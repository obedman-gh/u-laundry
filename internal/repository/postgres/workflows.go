package postgres

import (
	"context"
	"database/sql"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (s *session) LockService(ctx context.Context, id string) (repository.ServiceRecord, error) {
	var v domain.Service
	var version int64
	err := s.tx.QueryRowContext(ctx, `SELECT `+serviceColumns+`,version FROM public.services WHERE id=$1 FOR UPDATE`, id).Scan(&v.ID, &v.Name, &v.Unit, &v.UnitPrice, &v.Turnaround, &v.Active, &v.CreatedAt, &v.UpdatedAt, &version)
	if err != nil {
		return repository.ServiceRecord{}, dbError(ctx, "lock service", err)
	}
	if v.Validate() != nil || version < 1 {
		return repository.ServiceRecord{}, corrupt("lock service")
	}
	return repository.ServiceRecord{Service: v, Version: version}, nil
}
func (s *session) UpdateService(ctx context.Context, v domain.Service, expected int64) error {
	if err := v.Validate(); err != nil {
		return err
	}
	result, err := s.tx.ExecContext(ctx, `UPDATE public.services SET name=$1,unit=$2,unit_price=$3,turnaround_ns=$4,active=$5,updated_at=$6,version=version+1 WHERE id=$7 AND version=$8`, v.Name, v.Unit, v.UnitPrice, int64(v.Turnaround), v.Active, databaseTime(v.UpdatedAt), v.ID, expected)
	if err != nil {
		return dbError(ctx, "configure service", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "configure service count", err)
	}
	if n != 1 {
		return stale("configure service")
	}
	return nil
}
func (s *session) AddTracking(ctx context.Context, id string, hash [32]byte) error {
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.order_tracking_tokens(order_id,token_hash) VALUES ($1,$2)`, id, hash[:])
	return dbError(ctx, "register tracking", err)
}
func (s *session) GetTracking(ctx context.Context, hash [32]byte) (repository.TrackingRecord, error) {
	var v repository.TrackingRecord
	var ready, collected sql.NullTime
	err := s.tx.QueryRowContext(ctx, `SELECT o.status,o.estimated_ready_at,o.ready_at,o.collected_at FROM public.orders o JOIN public.order_tracking_tokens t ON t.order_id=o.id WHERE t.token_hash=$1`, hash[:]).Scan(&v.Status, &v.EstimatedReadyAt, &ready, &collected)
	if err != nil {
		return v, dbError(ctx, "get tracking", err)
	}
	v.ReadyAt, v.CollectedAt = ready.Time, collected.Time
	if !v.Status.Valid() || v.EstimatedReadyAt.IsZero() {
		return repository.TrackingRecord{}, corrupt("get tracking")
	}
	return v, nil
}
func (s *session) LedgerTotals(ctx context.Context, from, to time.Time) (repository.LedgerTotals, error) {
	var v repository.LedgerTotals
	err := s.tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount) FILTER (WHERE type='DEBIT'),0),COALESCE(SUM(amount) FILTER (WHERE type='KREDIT'),0) FROM public.ledger_entries WHERE occurred_at >= $1 AND occurred_at < $2`, from, to).Scan(&v.Inflow, &v.Outflow)
	if err != nil {
		return v, dbError(ctx, "generate report", err)
	}
	if v.Inflow < 0 || v.Outflow < 0 {
		return repository.LedgerTotals{}, corrupt("generate report")
	}
	return v, nil
}
