package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"
)

var shiftAt = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)

type shiftFixture struct {
	db       *sql.DB
	store    *postgres.Store
	shifts   *Shifts
	orders   *Orders
	payments *Payments
	rt       Runtime
	ctx      context.Context
}

func newShiftFixture(t *testing.T) shiftFixture {
	t.Helper()
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := postgres.New(db)
	hash, err := bcrypt.GenerateFromPassword([]byte("unused-synthetic-shift"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.WithinTransaction(ctx, func(tx repository.Tx) error {
		for id, role := range map[string]domain.Role{"owner": domain.Owner, "admin": domain.Admin, "cashier": domain.Cashier, "other": domain.Cashier} {
			if err := tx.AddStaff(ctx, domain.Staff{ID: id, Username: id, Name: "Staf Contoh", Role: role, Active: true, CreatedAt: shiftAt, UpdatedAt: shiftAt}, string(hash)); err != nil {
				return err
			}
		}
		c, err := domain.NewCustomer("customer", "Pelanggan Contoh", "", "", shiftAt)
		if err != nil {
			return err
		}
		if err = tx.AddCustomer(ctx, c); err != nil {
			return err
		}
		return tx.AddService(ctx, domain.Service{ID: "service", Name: "Cuci Contoh", Unit: domain.PerPiece, UnitPrice: 10000, Turnaround: time.Hour, Active: true, CreatedAt: shiftAt, UpdatedAt: shiftAt})
	}); err != nil {
		t.Fatal(err)
	}
	rt := Runtime{Clock: ClockFunc(func() time.Time { return shiftAt.Add(time.Hour) }), IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	return shiftFixture{db, store, NewShifts(store, rt), NewOrders(store, rt), NewPayments(store, rt), rt, ctx}
}
func (f shiftFixture) open(t *testing.T, actor, key string, opening domain.Money) ShiftView {
	t.Helper()
	v, err := f.shifts.Open(f.ctx, actor, ShiftOpenInput{Key: key, Opening: opening})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func (f shiftFixture) order(t *testing.T, actor, key string) string {
	t.Helper()
	o, err := f.orders.Create(f.ctx, actor, OrderInput{IdempotencyKey: key, CustomerID: "customer", Items: []domain.ItemRequest{{ServiceID: "service", Quantity: domain.Quantity{Pieces: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	return o.Order.ID()
}
func (f shiftFixture) pay(t *testing.T, actor, id, key string, amount domain.Money, method domain.PaymentMethod) domain.Payment {
	t.Helper()
	p, err := f.payments.Record(f.ctx, actor, PaymentInput{OrderID: id, IdempotencyKey: key, Amount: amount, Method: method})
	if err != nil {
		t.Fatal(err)
	}
	return p.Payment
}
func (f shiftFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(f.ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPostgresCashShifts(t *testing.T) {
	t.Run("concurrent_open_replay_and_unique_active", func(t *testing.T) {
		f := newShiftFixture(t)
		result := make([]ShiftView, 8)
		errs := concurrentOrderCommands(8, func(i int) error {
			var err error
			result[i], err = f.shifts.Open(f.ctx, "cashier", ShiftOpenInput{Key: "open", Opening: 100000})
			return err
		})
		fresh := 0
		for i, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
			if !result[i].Replayed {
				fresh++
			}
			if result[i].Shift.ID != result[0].Shift.ID {
				t.Fatal("duplicate shift")
			}
		}
		if fresh != 1 || f.count(t, `SELECT count(*) FROM audit_events WHERE action='shift_opened'`) != 1 {
			t.Fatal("open posted twice")
		}
		_, err := f.shifts.Open(f.ctx, "cashier", ShiftOpenInput{Key: "different"})
		requireDomain(t, err, domain.Conflict, "active_shift_exists")
		_, err = f.shifts.Open(f.ctx, "cashier", ShiftOpenInput{Key: "open", Opening: 1})
		requireDomain(t, err, domain.Conflict, "idempotency_conflict")
		f.open(t, "other", "open", 0)
		if err := f.store.WithinTransaction(f.ctx, func(tx repository.Tx) error {
			return tx.AddShift(f.ctx, domain.CashShift{ID: "duplicate", CashierID: "cashier", OpenKey: "sql", OpenedAt: shiftAt.Add(time.Hour)})
		}); !errors.Is(err, repository.ErrConflict) {
			t.Fatal("unique active constraint missing", err)
		}
	})
	t.Run("payment_association_cash_movements_refund_and_reconciliation", func(t *testing.T) {
		f := newShiftFixture(t)
		id := f.order(t, "owner", "order")
		_, err := f.payments.Record(f.ctx, "owner", PaymentInput{OrderID: id, IdempotencyKey: "cash", Amount: 3000, Method: domain.Cash})
		requireDomain(t, err, domain.Conflict, "active_shift_required")
		if f.count(t, `SELECT count(*) FROM payments`) != 0 {
			t.Fatal("missing shift left payment")
		}
		bank := f.pay(t, "owner", id, "bank", 2000, domain.BankTransfer)
		if bank.ShiftID != "" {
			t.Fatal("invented noncash shift")
		}
		v := f.open(t, "owner", "open", 10000)
		cash := f.pay(t, "owner", id, "cash", 3000, domain.Cash)
		qris := f.pay(t, "owner", id, "qris", 1000, domain.QRIS)
		if cash.ShiftID != v.Shift.ID || qris.ShiftID != v.Shift.ID {
			t.Fatal("receipt association missing")
		}
		for _, in := range []CashMoveInput{{ID: v.Shift.ID, Key: "in", Amount: 2000, Direction: domain.Debit, Notes: "Tambahan uang kecil"}, {ID: v.Shift.ID, Key: "out", Amount: 500, Direction: domain.Credit, Notes: "Pembelian kantong"}} {
			first, err := f.shifts.Move(f.ctx, "owner", in)
			if err != nil {
				t.Fatal(err)
			}
			again, err := f.shifts.Move(f.ctx, "owner", in)
			if err != nil || !again.Replayed || again.Movement.Entry.ID != first.Movement.Entry.ID {
				t.Fatal("movement duplicate", err)
			}
		}
		refund, err := f.payments.Refund(f.ctx, "owner", RefundInput{OrderID: id, PaymentID: cash.ID, IdempotencyKey: "refund", Reason: "Koreksi pembayaran"})
		if err != nil || refund.Refund.ShiftID != v.Shift.ID {
			t.Fatal("refund association", err)
		}
		view, err := f.shifts.Get(f.ctx, "owner", v.Shift.ID)
		if err != nil || view.Expected != 11500 || view.Totals.CashPayments != 3000 || view.Totals.CashRefunds != 3000 || view.Totals.QRISPayments != 1000 {
			t.Fatal(view, err)
		}
		_, err = f.shifts.Close(f.ctx, "owner", ShiftCloseInput{ID: v.Shift.ID, Key: "close", Physical: 11000})
		requireDomain(t, err, domain.Validation, "")
		closed, err := f.shifts.Close(f.ctx, "owner", ShiftCloseInput{ID: v.Shift.ID, Key: "close", Physical: 11000, Notes: "Selisih hasil hitung"})
		if err != nil || closed.Shift.Difference != -500 || closed.Shift.ClosedBy != "owner" {
			t.Fatal(closed, err)
		}
		replay, err := f.shifts.Close(f.ctx, "owner", ShiftCloseInput{ID: v.Shift.ID, Key: "close", Physical: 11000, Notes: "Selisih hasil hitung"})
		if err != nil || !replay.Replayed {
			t.Fatal(err)
		}
		_, err = f.shifts.Move(f.ctx, "owner", CashMoveInput{ID: v.Shift.ID, Key: "late", Amount: 1, Direction: domain.Debit, Notes: "Ditolak"})
		requireDomain(t, err, domain.Conflict, "shift_closed")
		p, err := f.payments.Record(f.ctx, "owner", PaymentInput{OrderID: id, IdempotencyKey: "cash", Amount: 3000, Method: domain.Cash})
		if err != nil || !p.Replayed || p.Payment.ShiftID != v.Shift.ID {
			t.Fatal("closed payment replay", err)
		}
		day, err := f.shifts.Daily(f.ctx, "owner", "2026-11-25")
		if err != nil || len(day.Shifts) != 1 || day.Unassigned.BankPayments != 2000 || day.Totals.BankPayments != 2000 || day.Totals.CashIn != 2000 {
			t.Fatal(day, err)
		}
		if _, err = f.db.ExecContext(f.ctx, `INSERT INTO ledger_entries(id,type,amount,description,recorded_by,occurred_at)VALUES('unknown-expense','KREDIT',500,'Metode historis tidak diketahui','owner',$1)`, shiftAt.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		day, err = f.shifts.Daily(f.ctx, "owner", "2026-11-25")
		if err != nil || day.Totals.UnknownOut != 500 || day.Unassigned.UnknownOut != 500 {
			t.Fatal("unclassified ledger omitted", err)
		}
		entries, err := f.shifts.Entries(f.ctx, "owner", v.Shift.ID, repository.Page{})
		if err != nil || len(entries) != 5 {
			t.Fatal("entries", len(entries), err)
		}
		f.open(t, "owner", "next", 11000)
		old, err := f.shifts.Open(f.ctx, "owner", ShiftOpenInput{Key: "open", Opening: 10000})
		if err != nil || !old.Replayed || old.Shift.IsOpen() {
			t.Fatal("old open retry reopened drawer", err)
		}
	})
	t.Run("ownership_and_replay_permission", func(t *testing.T) {
		f := newShiftFixture(t)
		v := f.open(t, "cashier", "open", 0)
		for _, actor := range []string{"other", "owner"} {
			_, err := f.shifts.Close(f.ctx, actor, ShiftCloseInput{ID: v.Shift.ID, Key: "close"})
			requireDomain(t, err, domain.Forbidden, "")
			_, err = f.shifts.Move(f.ctx, actor, CashMoveInput{ID: v.Shift.ID, Key: "cash", Amount: 1, Direction: domain.Debit, Notes: "Ditolak"})
			requireDomain(t, err, domain.Forbidden, "")
		}
		_, err := f.shifts.Get(f.ctx, "other", v.Shift.ID)
		requireDomain(t, err, domain.Forbidden, "")
		if _, err = f.shifts.Get(f.ctx, "owner", v.Shift.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = f.shifts.Get(f.ctx, "admin", v.Shift.ID); err != nil {
			t.Fatal(err)
		}
		day, err := f.shifts.Daily(f.ctx, "other", "2026-11-25")
		if err != nil || len(day.Shifts) != 0 {
			t.Fatal("cross cashier daily leak", err)
		}
		if _, err = f.db.ExecContext(f.ctx, `UPDATE staff SET active=false WHERE id='cashier'`); err != nil {
			t.Fatal(err)
		}
		_, err = f.shifts.Open(f.ctx, "cashier", ShiftOpenInput{Key: "open"})
		requireDomain(t, err, domain.Forbidden, "")
	})
	t.Run("refund_uses_current_drawer_and_preserves_closed_shift", func(t *testing.T) {
		f := newShiftFixture(t)
		first := f.open(t, "owner", "first", 0)
		id := f.order(t, "owner", "order")
		p := f.pay(t, "owner", id, "payment", 3000, domain.Cash)
		if _, err := f.shifts.Close(f.ctx, "owner", ShiftCloseInput{ID: first.Shift.ID, Key: "close", Physical: 3000}); err != nil {
			t.Fatal(err)
		}
		next := f.open(t, "owner", "next", 0)
		in := RefundInput{OrderID: id, PaymentID: p.ID, IdempotencyKey: "refund", Reason: "Koreksi pembayaran"}
		_, err := f.payments.Refund(f.ctx, "owner", in)
		requireDomain(t, err, domain.Conflict, "insufficient_cash")
		if _, err = f.shifts.Move(f.ctx, "owner", CashMoveInput{ID: next.Shift.ID, Key: "fund", Amount: 3000, Direction: domain.Debit, Notes: "Tambahan dana refund"}); err != nil {
			t.Fatal(err)
		}
		r, err := f.payments.Refund(f.ctx, "owner", in)
		if err != nil || r.Refund.ShiftID != next.Shift.ID {
			t.Fatal("refund assigned to historical shift", err)
		}
		closed, err := f.shifts.Get(f.ctx, "owner", first.Shift.ID)
		if err != nil || closed.Expected != 3000 || closed.Shift.Difference != 0 {
			t.Fatal("refund changed closed count", err)
		}
		current, err := f.shifts.Current(f.ctx, "owner")
		if err != nil || current == nil || current.Expected != 0 {
			t.Fatal("refund drawer balance", err)
		}
		replay, err := f.payments.Record(f.ctx, "owner", PaymentInput{OrderID: id, IdempotencyKey: "payment", Amount: 3000, Method: domain.Cash})
		if err != nil || !replay.Replayed || replay.Payment.ShiftID != first.Shift.ID {
			t.Fatal("retry reassigned payment", err)
		}
	})
	t.Run("cash_shortage_and_concurrent_distinct_movements", func(t *testing.T) {
		f := newShiftFixture(t)
		v := f.open(t, "cashier", "open", 1000)
		errs := concurrentOrderCommands(2, func(i int) error {
			_, err := f.shifts.Move(f.ctx, "cashier", CashMoveInput{ID: v.Shift.ID, Key: fmt.Sprintf("out-%d", i), Amount: 600, Direction: domain.Credit, Notes: "Setor uang"})
			return err
		})
		won := 0
		for _, err := range errs {
			if err == nil {
				won++
			} else {
				requireDomain(t, err, domain.Conflict, "insufficient_cash")
			}
		}
		v, err := f.shifts.Get(f.ctx, "cashier", v.Shift.ID)
		if err != nil || won != 1 || v.Expected != 400 {
			t.Fatal("lost cash update", v, err)
		}
	})
	t.Run("close_versus_payment_is_serialized", func(t *testing.T) {
		f := newShiftFixture(t)
		v := f.open(t, "cashier", "open", 0)
		id := f.order(t, "cashier", "order")
		errs := concurrentOrderCommands(2, func(i int) error {
			if i == 0 {
				_, err := f.payments.Record(f.ctx, "cashier", PaymentInput{OrderID: id, IdempotencyKey: "pay", Amount: 1000, Method: domain.Cash})
				return err
			}
			_, err := f.shifts.Close(f.ctx, "cashier", ShiftCloseInput{ID: v.Shift.ID, Key: "close", Physical: 0, Notes: "Hitungan saat penutupan"})
			return err
		})
		if errs[1] != nil {
			t.Fatal("close failed", errs)
		}
		closed, err := f.shifts.Get(f.ctx, "cashier", v.Shift.ID)
		if err != nil {
			t.Fatal(err)
		}
		if errs[0] == nil {
			if closed.Expected != 1000 || closed.Shift.Difference != -1000 {
				t.Fatal("payment omitted from close")
			}
		} else {
			requireDomain(t, errs[0], domain.Conflict, "active_shift_required")
			if closed.Expected != 0 {
				t.Fatal("failed payment affected close")
			}
		}
		if f.count(t, `SELECT count(*) FROM ledger_entries l JOIN cash_shifts s ON s.id=l.shift_id WHERE l.occurred_at>s.closed_at`) != 0 {
			t.Fatal("late ledger entry")
		}
	})
	t.Run("audit_failure_rolls_back_open_movement_close_and_key", func(t *testing.T) {
		f := newShiftFixture(t)
		broken := NewShifts(workflowFailureWork{UnitOfWork: f.store, stage: "audit"}, f.rt)
		if v, err := broken.Open(f.ctx, "cashier", ShiftOpenInput{Key: "open"}); err == nil || v.Shift.ID != "" || f.count(t, `SELECT count(*) FROM cash_shifts`) != 0 {
			t.Fatal("partial open", err)
		}
		v := f.open(t, "cashier", "open", 1000)
		in := CashMoveInput{ID: v.Shift.ID, Key: "move", Amount: 100, Direction: domain.Debit, Notes: "Tambahan kas"}
		if _, err := broken.Move(f.ctx, "cashier", in); err == nil || f.count(t, `SELECT count(*) FROM ledger_entries`) != 0 || f.count(t, `SELECT count(*) FROM shift_cash_movements`) != 0 {
			t.Fatal("partial move", err)
		}
		if _, err := f.shifts.Move(f.ctx, "cashier", in); err != nil {
			t.Fatal("key poisoned", err)
		}
		if _, err := broken.Close(f.ctx, "cashier", ShiftCloseInput{ID: v.Shift.ID, Key: "close", Physical: 1100}); err == nil {
			t.Fatal("audit error lost")
		}
		if current, err := f.shifts.Current(f.ctx, "cashier"); err != nil || current == nil {
			t.Fatal("partial close", err)
		}
		if _, err := f.shifts.Close(f.ctx, "cashier", ShiftCloseInput{ID: v.Shift.ID, Key: "close", Physical: 1100}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("repository_constraints_and_posting_required", func(t *testing.T) {
		f := newShiftFixture(t)
		v := f.open(t, "cashier", "open", 1000)
		violation := func(q string, args ...any) {
			t.Helper()
			_, err := f.db.ExecContext(f.ctx, q, args...)
			var pg *pq.Error
			if !errors.As(err, &pg) || pg.Code != "23514" {
				t.Fatal("expected database guard", err)
			}
		}
		violation(`UPDATE cash_shifts SET opening=opening+1 WHERE id=$1`, v.Shift.ID)
		violation(`DELETE FROM cash_shifts WHERE id=$1`, v.Shift.ID)
		violation(`INSERT INTO ledger_entries(id,type,amount,description,recorded_by,occurred_at,method)VALUES('missing','DEBIT',1,'Contoh','cashier',$1,'cash')`, shiftAt.Add(time.Hour))
		violation(`INSERT INTO ledger_entries(id,shift_id,type,amount,description,recorded_by,occurred_at,method)VALUES('missing-key',$1,'DEBIT',1,'Contoh','cashier',$2,'cash')`, v.Shift.ID, shiftAt.Add(time.Hour))
		violation(`INSERT INTO ledger_entries(id,shift_id,type,amount,description,recorded_by,occurred_at,method)VALUES('other',$1,'DEBIT',1,'Contoh','other',$2,'cash')`, v.Shift.ID, shiftAt.Add(time.Hour))
		violation(`UPDATE cash_shifts SET closed_at=$2,closed_by='cashier',close_key='sql',system_cash=999,physical_cash=999,difference=0 WHERE id=$1`, v.Shift.ID, shiftAt.Add(time.Hour))
		if f.count(t, `SELECT count(*) FROM ledger_entries`) != 0 {
			t.Fatal("constraint left postings")
		}
		if _, err := f.shifts.Close(f.ctx, "cashier", ShiftCloseInput{ID: v.Shift.ID, Key: "close", Physical: 1000}); err != nil {
			t.Fatal(err)
		}
		violation(`UPDATE cash_shifts SET closing_notes='rewrite' WHERE id=$1`, v.Shift.ID)
		violation(`INSERT INTO ledger_entries(id,shift_id,type,amount,description,recorded_by,occurred_at,method)VALUES('late',$1,'DEBIT',1,'Contoh','cashier',$2,'cash')`, v.Shift.ID, shiftAt.Add(time.Hour))
	})
	t.Run("daily_wita_boundaries_and_default", func(t *testing.T) {
		f := newShiftFixture(t)
		for i, at := range []time.Time{shiftAt.Add(-8*time.Hour - time.Microsecond), shiftAt.Add(-8 * time.Hour), shiftAt.Add(16 * time.Hour)} {
			rt := f.rt
			rt.Clock = ClockFunc(func() time.Time { return at })
			actor := []string{"owner", "cashier", "other"}[i]
			v, err := NewShifts(f.store, rt).Open(f.ctx, actor, ShiftOpenInput{Key: "day"})
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				if _, err = NewShifts(f.store, rt).Close(f.ctx, actor, ShiftCloseInput{ID: v.Shift.ID, Key: "prior-day-close", Physical: 0}); err != nil {
					t.Fatal(err)
				}
			}
		}
		v, err := f.shifts.Daily(f.ctx, "owner", "")
		if err != nil || v.Date != "2026-11-25" || len(v.Shifts) != 1 || v.Shifts[0].Shift.CashierID != "cashier" {
			t.Fatal(v, err)
		}
		overnight := f.rt
		overnight.Clock = ClockFunc(func() time.Time { return shiftAt.Add(-9 * time.Hour) })
		if _, err = NewShifts(f.store, overnight).Open(f.ctx, "admin", ShiftOpenInput{Key: "overnight"}); err != nil {
			t.Fatal(err)
		}
		v, err = f.shifts.Daily(f.ctx, "owner", "2026-11-25")
		if err != nil || len(v.Shifts) != 2 || v.Shifts[0].Shift.CashierID != "admin" {
			t.Fatal("overnight shift omitted", v, err)
		}
		_, err = f.shifts.Daily(f.ctx, "owner", "2026-02-30")
		requireDomain(t, err, domain.Validation, "")
	})
}
