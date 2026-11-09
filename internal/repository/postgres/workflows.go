package postgres

import (
	"context"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (s *session) LockService(ctx context.Context, id string) (repository.ServiceRecord, error) {
	var v domain.Service
	var version int64
	err := s.tx.QueryRowContext(ctx, `SELECT `+serviceColumns+`,version FROM public.services WHERE id=$1 FOR UPDATE`, id).Scan(&v.ID, &v.Name, &v.Unit, &v.UnitPrice, &v.Turnaround, &v.Active, &v.CreatedAt, &v.UpdatedAt, &v.DisplayOrder, &version)
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
	result, err := s.tx.ExecContext(ctx, `UPDATE public.services SET name=$1,unit=$2,unit_price=$3,turnaround_ns=$4,active=$5,updated_at=$6,version=version+1,display_order=$9 WHERE id=$7 AND version=$8`, v.Name, v.Unit, v.UnitPrice, int64(v.Turnaround), v.Active, databaseTime(v.UpdatedAt), v.ID, expected, v.DisplayOrder)
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
func (s *session) AddTracking(ctx context.Context, id string, hash [32]byte, expires time.Time) error {
	if expires.IsZero() {
		return corrupt("register tracking expiry")
	}
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.order_tracking_tokens(order_id,token_hash,expires_at) VALUES ($1,$2,$3)`, id, hash[:], databaseTime(expires))
	return dbError(ctx, "register tracking", err)
}
func (s *session) GetTracking(ctx context.Context, hash [32]byte, at time.Time) (repository.TrackingRecord, error) {
	var v repository.TrackingRecord
	err := s.tx.QueryRowContext(ctx, `SELECT o.status,o.estimated_ready_at FROM public.orders o JOIN public.order_tracking_tokens t ON t.order_id=o.id WHERE t.token_hash=$1 AND t.expires_at>$2`, hash[:], databaseTime(at)).Scan(&v.Status, &v.EstimatedReadyAt)
	if err != nil {
		return v, dbError(ctx, "get tracking", err)
	}
	if !v.Status.Valid() || v.EstimatedReadyAt.IsZero() {
		return repository.TrackingRecord{}, corrupt("get tracking")
	}
	return v, nil
}
func (s *session) ReplaceTracking(ctx context.Context, id string, hash [32]byte, expires time.Time, expected int64) error {
	// The caller holds the order lock; the version predicate also protects direct
	// repository callers. A collision aborts the entire transaction, never retries.
	result, err := s.tx.ExecContext(ctx, `UPDATE public.orders SET version=version+1 WHERE id=$1 AND version=$2`, id, expected)
	if err != nil {
		return dbError(ctx, "rotate tracking", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "rotate tracking count", err)
	}
	if n != 1 {
		return stale("rotate tracking")
	}
	if expires.IsZero() {
		return corrupt("rotate tracking expiry")
	}
	// A repeated generator output must fail instead of claiming revocation.
	var exists bool
	if err := s.tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM public.order_tracking_tokens WHERE token_hash=$1)`, hash[:]).Scan(&exists); err != nil {
		return dbError(ctx, "rotate tracking collision", err)
	}
	if exists {
		return stale("rotate tracking collision")
	}
	_, err = s.tx.ExecContext(ctx, `INSERT INTO public.order_tracking_tokens(order_id,token_hash,expires_at) VALUES($1,$2,$3) ON CONFLICT(order_id) DO UPDATE SET token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at`, id, hash[:], databaseTime(expires))
	return dbError(ctx, "replace tracking", err)
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
