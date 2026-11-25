package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

const shiftColumns = `id,cashier_id,open_key,opening,opening_notes,opened_at,closed_at,closed_by,close_key,system_cash,physical_cash,difference,closing_notes`

func scanShift(ctx context.Context, row scanner) (domain.CashShift, error) {
	var s domain.CashShift
	var at sql.NullTime
	var actor, key sql.NullString
	var system, physical, difference sql.NullInt64
	if err := row.Scan(&s.ID, &s.CashierID, &s.OpenKey, &s.Opening, &s.OpeningNotes, &s.OpenedAt, &at, &actor, &key, &system, &physical, &difference, &s.ClosingNotes); err != nil {
		return s, dbError(ctx, "read shift", err)
	}
	s.ClosedAt, s.ClosedBy, s.CloseKey = at.Time, actor.String, key.String
	s.System, s.Physical, s.Difference = domain.Money(system.Int64), domain.Money(physical.Int64), domain.Money(difference.Int64)
	if s.Validate() != nil {
		return domain.CashShift{}, corrupt("read shift")
	}
	return s, nil
}
func (s *session) GetShift(ctx context.Context, id string) (domain.CashShift, error) {
	return scanShift(ctx, s.tx.QueryRowContext(ctx, `SELECT `+shiftColumns+` FROM public.cash_shifts WHERE id=$1`, id))
}
func (s *session) LockShift(ctx context.Context, id string) (domain.CashShift, error) {
	return scanShift(ctx, s.tx.QueryRowContext(ctx, `SELECT `+shiftColumns+` FROM public.cash_shifts WHERE id=$1 FOR UPDATE`, id))
}
func (s *session) ActiveShift(ctx context.Context, actor string) (domain.CashShift, error) {
	return scanShift(ctx, s.tx.QueryRowContext(ctx, `SELECT `+shiftColumns+` FROM public.cash_shifts WHERE cashier_id=$1 AND closed_at IS NULL`, actor))
}
func (s *session) FindShiftByOpenKey(ctx context.Context, actor, key string) (domain.CashShift, error) {
	return scanShift(ctx, s.tx.QueryRowContext(ctx, `SELECT `+shiftColumns+` FROM public.cash_shifts WHERE cashier_id=$1 AND open_key=$2`, actor, key))
}
func (s *session) LockShiftOwner(ctx context.Context, actor string) error {
	_, err := s.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('cash-shift:'||$1,0))`, actor)
	return dbError(ctx, "lock shift owner", err)
}
func (s *session) LockPostingShift(ctx context.Context, actor string, method domain.PaymentMethod) (string, error) {
	v, err := scanShift(ctx, s.tx.QueryRowContext(ctx, `SELECT `+shiftColumns+` FROM public.cash_shifts WHERE cashier_id=$1 AND closed_at IS NULL FOR UPDATE`, actor))
	if errors.Is(err, repository.ErrNotFound) {
		if method == domain.Cash {
			return "", &domain.Error{Kind: domain.Conflict, Code: "active_shift_required", Field: "shift", Message: "Buka shift kas sebelum mencatat transaksi tunai."}
		}
		return "", nil
	}
	return v.ID, err
}
func (s *session) AddShift(ctx context.Context, v domain.CashShift) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if !v.IsOpen() {
		return stale("open closed shift")
	}
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.cash_shifts(id,cashier_id,open_key,opening,opening_notes,opened_at)VALUES($1,$2,$3,$4,$5,$6)`, v.ID, v.CashierID, v.OpenKey, v.Opening, v.OpeningNotes, databaseTime(v.OpenedAt))
	return dbError(ctx, "open shift", err)
}
func (s *session) CloseShift(ctx context.Context, v domain.CashShift) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.IsOpen() {
		return stale("close open shift")
	}
	result, err := s.tx.ExecContext(ctx, `UPDATE public.cash_shifts SET closed_at=$1,closed_by=$2,close_key=$3,system_cash=$4,physical_cash=$5,difference=$6,closing_notes=$7 WHERE id=$8 AND closed_at IS NULL`, databaseTime(v.ClosedAt), v.ClosedBy, v.CloseKey, v.System, v.Physical, v.Difference, v.ClosingNotes, v.ID)
	if err != nil {
		return dbError(ctx, "close shift", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "close shift count", err)
	}
	if n != 1 {
		return stale("close shift")
	}
	return nil
}

const shiftTotalsSQL = `SELECT
 COALESCE(sum(amount) FILTER(WHERE method='cash' AND payment_id IS NOT NULL),0),
 COALESCE(sum(amount) FILTER(WHERE method='cash' AND refund_id IS NOT NULL),0),
 COALESCE(sum(amount) FILTER(WHERE method='cash' AND payment_id IS NULL AND refund_id IS NULL AND type='DEBIT'),0),
 COALESCE(sum(amount) FILTER(WHERE method='cash' AND payment_id IS NULL AND refund_id IS NULL AND type='KREDIT'),0),
 COALESCE(sum(amount) FILTER(WHERE method='bank_transfer' AND payment_id IS NOT NULL),0),
 COALESCE(sum(amount) FILTER(WHERE method='bank_transfer' AND refund_id IS NOT NULL),0),
 COALESCE(sum(amount) FILTER(WHERE method='qris' AND payment_id IS NOT NULL),0),
 COALESCE(sum(amount) FILTER(WHERE method='qris' AND refund_id IS NOT NULL),0),
 COALESCE(sum(amount) FILTER(WHERE method IS NULL AND type='DEBIT'),0),
 COALESCE(sum(amount) FILTER(WHERE method IS NULL AND type='KREDIT'),0)
 FROM public.ledger_entries `

func scanShiftTotals(ctx context.Context, row scanner) (domain.ShiftTotals, error) {
	var v domain.ShiftTotals
	err := row.Scan(&v.CashPayments, &v.CashRefunds, &v.CashIn, &v.CashOut, &v.BankPayments, &v.BankRefunds, &v.QRISPayments, &v.QRISRefunds, &v.UnknownIn, &v.UnknownOut)
	return v, dbError(ctx, "shift totals", err)
}
func (s *session) ShiftTotals(ctx context.Context, id string) (domain.ShiftTotals, error) {
	return scanShiftTotals(ctx, s.tx.QueryRowContext(ctx, shiftTotalsSQL+`WHERE shift_id=$1`, id))
}
func (s *session) DailyShiftTotals(ctx context.Context, from, to time.Time, actor string, unassigned bool) (domain.ShiftTotals, error) {
	return scanShiftTotals(ctx, s.tx.QueryRowContext(ctx, shiftTotalsSQL+`WHERE occurred_at >= $1 AND occurred_at < $2 AND ($3::text='' OR recorded_by=$3) AND (NOT $4 OR shift_id IS NULL)`, from, to, actor, unassigned))
}
func (s *session) ShiftsForDay(ctx context.Context, from, to time.Time, actor string) ([]domain.CashShift, error) {
	return queryAll(ctx, s.tx, "daily shifts", `SELECT `+shiftColumns+` FROM public.cash_shifts WHERE opened_at < $2 AND (closed_at IS NULL OR closed_at >= $1) AND ($3::text='' OR cashier_id=$3) ORDER BY opened_at,id`, func(r *sql.Rows) (domain.CashShift, error) { return scanShift(ctx, r) }, from, to, actor)
}
func (s *session) ShiftEntries(ctx context.Context, id string, page repository.Page) ([]domain.LedgerEntry, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "shift entries", `SELECT `+ledgerColumns+` FROM public.ledger_entries WHERE shift_id=$1 ORDER BY occurred_at,id LIMIT $2 OFFSET $3`, func(r *sql.Rows) (domain.LedgerEntry, error) { return scanLedger(ctx, r) }, id, p.Limit, p.Offset)
}
func (s *session) FindCashMovement(ctx context.Context, id, key string) (domain.CashMovement, error) {
	e, err := scanLedger(ctx, s.tx.QueryRowContext(ctx, `SELECT `+ledgerColumns+` FROM public.ledger_entries WHERE id=(SELECT ledger_id FROM public.shift_cash_movements WHERE shift_id=$1 AND idempotency_key=$2)`, id, key))
	return domain.CashMovement{Entry: e, Key: key}, err
}
func (s *session) AddCashMovement(ctx context.Context, m domain.CashMovement) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := s.insertLedger(ctx, m.Entry); err != nil {
		return err
	}
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.shift_cash_movements(ledger_id,shift_id,idempotency_key)VALUES($1,$2,$3)`, m.Entry.ID, m.Entry.ShiftID, m.Key)
	return dbError(ctx, "record cash movement", err)
}
