package postgres

import (
	"context"
	"database/sql"
	"errors"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

const paymentColumns = `id,order_id,idempotency_key,recorded_by,amount,method,received_at`

func scanPayment(ctx context.Context, row scanner) (domain.Payment, error) {
	var p domain.Payment
	if err := row.Scan(&p.ID, &p.OrderID, &p.IdempotencyKey, &p.RecordedBy, &p.Amount, &p.Method, &p.ReceivedAt); err != nil {
		return p, dbError(ctx, "read payment", err)
	}
	if err := p.Validate(); err != nil {
		return domain.Payment{}, corrupt("read payment")
	}
	return p, nil
}
func (s *session) FindPaymentByKey(ctx context.Context, orderID, key string) (domain.Payment, error) {
	return scanPayment(ctx, s.tx.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM public.payments WHERE order_id=$1 AND idempotency_key=$2`, orderID, key))
}
func (s *session) RecordPayment(ctx context.Context, p domain.Payment, ledgerID string) (repository.PaymentResult, error) {
	p.ReceivedAt = databaseTime(p.ReceivedAt)
	if err := p.Validate(); err != nil {
		return repository.PaymentResult{}, err
	}
	stored, err := s.LockOrder(ctx, p.OrderID)
	if err != nil {
		return repository.PaymentResult{}, err
	}
	existing, err := s.FindPaymentByKey(ctx, p.OrderID, p.IdempotencyKey)
	if err == nil {
		// IDs/time are server-generated; retry compares the business payload.
		if existing.Amount != p.Amount || existing.Method != p.Method || existing.RecordedBy != p.RecordedBy {
			return repository.PaymentResult{}, stale("replay payment")
		}
		return repository.PaymentResult{Payment: existing, Replayed: true}, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return repository.PaymentResult{}, err
	}
	if err := stored.Order.RecordPayment(p); err != nil {
		return repository.PaymentResult{}, err
	}
	ledger, err := domain.NewPaymentLedger(ledgerID, p)
	if err != nil {
		return repository.PaymentResult{}, err
	}
	_, err = s.tx.ExecContext(ctx, `INSERT INTO public.payments (`+paymentColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7)`, p.ID, p.OrderID, p.IdempotencyKey, p.RecordedBy, p.Amount, p.Method, p.ReceivedAt)
	if err != nil {
		return repository.PaymentResult{}, dbError(ctx, "record payment", err)
	}
	if err := s.insertLedger(ctx, ledger); err != nil {
		return repository.PaymentResult{}, err
	}
	result, err := s.tx.ExecContext(ctx, `UPDATE public.orders SET updated_at=$1,version=version+1 WHERE id=$2 AND version=$3`, p.ReceivedAt, p.OrderID, stored.Version)
	if err != nil {
		return repository.PaymentResult{}, dbError(ctx, "update payment version", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return repository.PaymentResult{}, dbError(ctx, "update payment count", err)
	}
	if count != 1 {
		return repository.PaymentResult{}, stale("update payment version")
	}
	return repository.PaymentResult{Payment: p}, nil
}
func (s *session) insertLedger(ctx context.Context, e domain.LedgerEntry) error {
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.ledger_entries (id,order_id,payment_id,type,amount,description,recorded_by,occurred_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, e.ID, nullString(e.OrderID), nullString(e.PaymentID), e.Type, e.Amount, e.Description, e.RecordedBy, databaseTime(e.OccurredAt))
	return dbError(ctx, "record ledger", err)
}
func (s *session) RecordExpense(ctx context.Context, e domain.LedgerEntry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Type != domain.Credit || e.PaymentID != "" || e.OrderID != "" {
		return &domain.Error{Kind: domain.Validation, Code: "invalid_expense", Field: "ledger", Message: "Pengeluaran operasional harus KREDIT tanpa relasi pembayaran atau pesanan."}
	}
	return s.insertLedger(ctx, e)
}
func (s *session) LedgerForOrder(ctx context.Context, id string, page repository.Page) ([]domain.LedgerEntry, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "read ledger", `SELECT id,order_id,payment_id,type,amount,description,recorded_by,occurred_at FROM public.ledger_entries WHERE order_id=$1 ORDER BY occurred_at,id LIMIT $2 OFFSET $3`, func(row *sql.Rows) (domain.LedgerEntry, error) {
		var e domain.LedgerEntry
		var orderID, paymentID sql.NullString
		if err := row.Scan(&e.ID, &orderID, &paymentID, &e.Type, &e.Amount, &e.Description, &e.RecordedBy, &e.OccurredAt); err != nil {
			return e, dbError(ctx, "scan ledger", err)
		}
		e.OrderID, e.PaymentID = orderID.String, paymentID.String
		if err := e.Validate(); err != nil {
			return e, corrupt("read ledger")
		}
		return e, nil
	}, id, p.Limit, p.Offset)
}
