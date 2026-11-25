package postgres

import (
	"context"
	"database/sql"
	"errors"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

const refundColumns = `id,order_id,payment_id,idempotency_key,amount,method,recorded_by,refunded_at,reason,COALESCE((SELECT l.shift_id FROM public.ledger_entries l WHERE l.refund_id=refunds.id),'')`

func scanRefund(ctx context.Context, row scanner) (domain.Refund, error) {
	var r domain.Refund
	if err := row.Scan(&r.ID, &r.OrderID, &r.PaymentID, &r.IdempotencyKey, &r.Amount, &r.Method, &r.RecordedBy, &r.RefundedAt, &r.Reason, &r.ShiftID); err != nil {
		return r, dbError(ctx, "read refund", err)
	}
	if r.Validate() != nil {
		return domain.Refund{}, corrupt("read refund")
	}
	return r, nil
}
func (s *session) FindRefundByKey(ctx context.Context, order, key string) (domain.Refund, error) {
	return scanRefund(ctx, s.tx.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM public.refunds WHERE order_id=$1 AND idempotency_key=$2`, order, key))
}
func (s *session) PaymentsForOrder(ctx context.Context, id string, page repository.Page) ([]domain.Payment, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "list payments", `SELECT `+paymentColumns+` FROM public.payments WHERE order_id=$1 ORDER BY received_at,id LIMIT $2 OFFSET $3`, func(row *sql.Rows) (domain.Payment, error) { return scanPayment(ctx, row) }, id, p.Limit, p.Offset)
}
func (s *session) RefundsForOrder(ctx context.Context, id string, page repository.Page) ([]domain.Refund, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "list refunds", `SELECT `+refundColumns+` FROM public.refunds WHERE order_id=$1 ORDER BY refunded_at,id LIMIT $2 OFFSET $3`, func(row *sql.Rows) (domain.Refund, error) { return scanRefund(ctx, row) }, id, p.Limit, p.Offset)
}

func (s *session) RecordRefund(ctx context.Context, r domain.Refund, ledgerID string) (repository.RefundResult, error) {
	r.RefundedAt = databaseTime(r.RefundedAt)
	if err := r.Validate(); err != nil {
		return repository.RefundResult{}, err
	}
	o, err := s.LockOrder(ctx, r.OrderID)
	if err != nil {
		return repository.RefundResult{}, err
	}
	existing, err := s.FindRefundByKey(ctx, r.OrderID, r.IdempotencyKey)
	if err == nil {
		if existing.PaymentID != r.PaymentID || existing.RecordedBy != r.RecordedBy || existing.Reason != r.Reason || existing.Amount != r.Amount || existing.Method != r.Method {
			return repository.RefundResult{}, stale("replay refund")
		}
		return repository.RefundResult{Refund: existing, Replayed: true}, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return repository.RefundResult{}, err
	}
	if err := o.Order.RefundPayment(r); err != nil {
		return repository.RefundResult{}, err
	}
	r.ShiftID, err = s.LockPostingShift(ctx, r.RecordedBy, r.Method)
	if err != nil {
		return repository.RefundResult{}, err
	}
	ledger, err := domain.NewRefundLedger(ledgerID, r)
	if err != nil {
		return repository.RefundResult{}, err
	}
	if _, err = s.tx.ExecContext(ctx, `INSERT INTO public.refunds (id,order_id,payment_id,idempotency_key,amount,method,recorded_by,refunded_at,reason)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.ID, r.OrderID, r.PaymentID, r.IdempotencyKey, r.Amount, r.Method, r.RecordedBy, r.RefundedAt, r.Reason); err != nil {
		return repository.RefundResult{}, dbError(ctx, "record refund", err)
	}
	if err := s.insertLedger(ctx, ledger); err != nil {
		return repository.RefundResult{}, err
	}
	result, err := s.tx.ExecContext(ctx, `UPDATE public.orders SET updated_at=$1,version=version+1 WHERE id=$2 AND version=$3`, r.RefundedAt, r.OrderID, o.Version)
	if err != nil {
		return repository.RefundResult{}, dbError(ctx, "update refund version", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return repository.RefundResult{}, dbError(ctx, "update refund count", err)
	}
	if n != 1 {
		return repository.RefundResult{}, stale("update refund version")
	}
	return repository.RefundResult{Refund: r}, nil
}
