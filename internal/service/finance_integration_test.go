package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"
)

type refundFailureWork struct {
	repository.UnitOfWork
	stage string
}
type refundFailureTx struct {
	repository.Tx
	stage string
}

func (w refundFailureWork) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	return w.UnitOfWork.WithinTransaction(ctx, func(tx repository.Tx) error { return fn(refundFailureTx{tx, w.stage}) })
}
func (tx refundFailureTx) RecordRefund(ctx context.Context, r domain.Refund, ledgerID string) (repository.RefundResult, error) {
	if tx.stage == "ledger" {
		ledgerID = "deliberate-collision"
	}
	result, err := tx.Tx.RecordRefund(ctx, r, ledgerID)
	if err == nil && tx.stage == "after-posting" {
		return repository.RefundResult{}, failure
	}
	return result, err
}
func (tx refundFailureTx) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	if err := tx.Tx.AppendAudit(ctx, e); err != nil {
		return err
	}
	if tx.stage == "audit" {
		return failure
	}
	return nil
}

func TestPostgresFinancePaymentsAndRefunds(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	base := postgres.New(db)
	owner := domain.Staff{ID: "finance-owner", Username: "finance.owner", Name: "Owner Contoh", Role: domain.Owner, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	cashier := owner
	cashier.ID = "finance-cashier"
	cashier.Username = "finance.cashier"
	cashier.Role = domain.Cashier
	c, err := domain.NewCustomer("finance-customer", "Pelanggan Contoh", "", "", fixed)
	if err != nil {
		t.Fatal(err)
	}
	svc := domain.Service{ID: "finance-service", Name: "Cuci Contoh", Unit: domain.PerPiece, UnitPrice: 10000, Turnaround: time.Hour, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	hash, err := bcrypt.GenerateFromPassword([]byte("unused-synthetic-finance"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.WithinTransaction(ctx, func(tx repository.Tx) error {
		if err := tx.AddStaff(ctx, owner, string(hash)); err != nil {
			return err
		}
		if err := tx.AddStaff(ctx, cashier, string(hash)); err != nil {
			return err
		}
		if err := tx.AddCustomer(ctx, c); err != nil {
			return err
		}
		return tx.AddService(ctx, svc)
	}); err != nil {
		t.Fatal(err)
	}
	rt := Runtime{Clock: ClockFunc(func() time.Time { return fixed.Add(time.Hour) }), IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	for _, actor := range []string{owner.ID, cashier.ID} {
		if err := base.WithinTransaction(ctx, func(tx repository.Tx) error {
			return tx.AddShift(ctx, domain.CashShift{ID: "shift-" + actor, CashierID: actor, OpenKey: "fixture", Opening: 1000000, OpenedAt: fixed})
		}); err != nil {
			t.Fatal(err)
		}
	}
	orders, payments := NewOrders(base, rt), NewPayments(base, rt)
	create := func(key string) string {
		t.Helper()
		o, err := orders.Create(ctx, cashier.ID, OrderInput{IdempotencyKey: key, CustomerID: c.ID, Items: []domain.ItemRequest{{ServiceID: svc.ID, Quantity: domain.Quantity{Pieces: 1}}}})
		if err != nil {
			t.Fatal(err)
		}
		return o.Order.ID()
	}
	pay := func(id, key string, amount domain.Money, method domain.PaymentMethod) domain.Payment {
		t.Helper()
		r, err := payments.Record(ctx, cashier.ID, PaymentInput{OrderID: id, IdempotencyKey: key, Amount: amount, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		return r.Payment
	}
	read := func(id string) repository.OrderRecord {
		t.Helper()
		r, err := orders.Get(ctx, owner.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	refundInput := func(p domain.Payment, key string) RefundInput {
		return RefundInput{OrderID: p.OrderID, PaymentID: p.ID, IdempotencyKey: key, Reason: "Koreksi pembayaran"}
	}
	t.Run("partial_mixed_methods_refund_repay_and_history", func(t *testing.T) {
		id := create("partial-mixed")
		p := pay(id, "cash-part", 3000, domain.Cash)
		pay(id, "noncash-part", 2000, domain.QRIS)
		if r := read(id); r.Order.Paid() != 5000 || r.Order.Balance() != 5000 || r.Order.PaymentStatus() != domain.PartiallyPaid || r.Order.Status() != domain.Received {
			t.Fatal("invalid partial balance")
		}
		_, err := payments.Record(ctx, cashier.ID, PaymentInput{OrderID: id, IdempotencyKey: "over", Amount: 5001, Method: domain.Cash})
		requireDomain(t, err, domain.Conflict, "overpayment")
		refund, err := payments.Refund(ctx, owner.ID, refundInput(p, "refund-cash"))
		if err != nil || refund.Refund.Amount != 3000 || refund.Refund.Method != domain.Cash {
			t.Fatal(refund, err)
		}
		if r := read(id); r.Order.Paid() != 2000 || r.Order.Balance() != 8000 {
			t.Fatal("refund balance")
		}
		replay, err := payments.Record(ctx, cashier.ID, PaymentInput{OrderID: id, IdempotencyKey: p.IdempotencyKey, Amount: p.Amount, Method: p.Method})
		if err != nil || !replay.Replayed || replay.Payment.Status != domain.PaymentRefunded {
			t.Fatal("refunded payment retry posted again", err)
		}
		pay(id, "repay", 8000, domain.BankTransfer)
		if r := read(id); r.Order.Paid() != 10000 || r.Order.PaymentStatus() != domain.Paid {
			t.Fatal("repayment balance")
		}
		summaries, err := orders.List(ctx, owner.ID, repository.OrderFilter{CustomerID: c.ID})
		if err != nil || len(summaries) != 1 || summaries[0].Paid != 10000 {
			t.Fatal("order summary did not exclude refunded receipt", err)
		}
		report, err := NewReports(base, rt).Generate(ctx, owner.ID, ReportPeriod{From: fixed, To: fixed.Add(2 * time.Hour)})
		if err != nil || report.Inflow != 13000 || report.Outflow != 3000 || report.Balance != 10000 {
			t.Fatal("report omitted refund or original receipt", report, err)
		}
		list, err := payments.List(ctx, owner.ID, id, repository.Page{})
		if err != nil || len(list) != 3 || list[0].ID != p.ID && list[1].ID != p.ID && list[2].ID != p.ID {
			t.Fatal("history", err)
		}
		refunds, err := payments.Refunds(ctx, owner.ID, id, repository.Page{})
		if err != nil || len(refunds) != 1 || refunds[0].PaymentID != p.ID {
			t.Fatal("refund history", err)
		}
		if err := base.View(ctx, func(r repository.Readers) error {
			rows, err := r.LedgerForOrder(ctx, id, repository.Page{})
			if err != nil {
				return err
			}
			var net domain.Money
			for _, l := range rows {
				if l.Method == "" {
					t.Fatal("method missing")
				}
				if l.Type == domain.Debit {
					net += l.Amount
				} else {
					net -= l.Amount
				}
			}
			if len(rows) != 4 || net != 10000 {
				t.Fatal("cash ledger mismatch")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("concurrent_duplicate_payment_and_refund", func(t *testing.T) {
		id := create("duplicate-finance")
		results := make([]repository.PaymentResult, 8)
		errs := concurrentOrderCommands(8, func(i int) error {
			var err error
			results[i], err = payments.Record(ctx, cashier.ID, PaymentInput{OrderID: id, IdempotencyKey: "same", Amount: 10000, Method: domain.QRIS})
			return err
		})
		fresh := 0
		for i, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
			if !results[i].Replayed {
				fresh++
			}
			if results[i].Payment.ID != results[0].Payment.ID {
				t.Fatal("duplicate payment")
			}
		}
		if fresh != 1 {
			t.Fatal("payment applied twice")
		}
		refResults := make([]repository.RefundResult, 8)
		in := refundInput(results[0].Payment, "same-refund")
		errs = concurrentOrderCommands(8, func(i int) error { var err error; refResults[i], err = payments.Refund(ctx, owner.ID, in); return err })
		fresh = 0
		for i, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
			if !refResults[i].Replayed {
				fresh++
			}
			if refResults[i].Refund.ID != refResults[0].Refund.ID {
				t.Fatal("duplicate refund")
			}
		}
		if fresh != 1 || read(id).Order.Paid() != 0 || count(`SELECT count(*) FROM ledger_entries WHERE order_id=$1`, id) != 2 || count(`SELECT count(*) FROM audit_events WHERE entity_id=$1 AND action='payment_refunded'`, refResults[0].Refund.ID) != 1 {
			t.Fatal("refund double posted")
		}
		in.Reason = "Alasan berbeda"
		_, err := payments.Refund(ctx, owner.ID, in)
		requireDomain(t, err, domain.Conflict, "idempotency_conflict")
	})
	t.Run("different_keys_cannot_overpay_or_double_refund", func(t *testing.T) {
		id := create("different-keys")
		errs := concurrentOrderCommands(2, func(i int) error {
			_, err := payments.Record(ctx, cashier.ID, PaymentInput{OrderID: id, IdempotencyKey: fmt.Sprintf("pay-%d", i), Amount: 6000, Method: domain.Cash})
			return err
		})
		won := 0
		for _, err := range errs {
			if err == nil {
				won++
			} else {
				requireDomain(t, err, domain.Conflict, "overpayment")
			}
		}
		if won != 1 {
			t.Fatal("overpayment race")
		}
		p := read(id).Order.Payments()[0]
		errs = concurrentOrderCommands(2, func(i int) error {
			_, err := payments.Refund(ctx, owner.ID, refundInput(p, fmt.Sprintf("refund-%d", i)))
			return err
		})
		won = 0
		for _, err := range errs {
			if err == nil {
				won++
			} else {
				requireDomain(t, err, domain.Conflict, "already_refunded")
			}
		}
		if won != 1 || read(id).Order.Paid() != 0 {
			t.Fatal("refund race")
		}
	})
	t.Run("paid_cancel_guard_then_full_refunds_cancel", func(t *testing.T) {
		id := create("refund-cancel")
		p := pay(id, "deposit", 4000, domain.BankTransfer)
		_, err := orders.Cancel(ctx, owner.ID, id, 2, "Permintaan pelanggan", "before-refund")
		requireDomain(t, err, domain.Conflict, "payment_exists")
		_, err = payments.Refund(ctx, cashier.ID, refundInput(p, "denied"))
		requireDomain(t, err, domain.Forbidden, "")
		r, err := payments.Refund(ctx, owner.ID, refundInput(p, "refund-bank"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := orders.Cancel(ctx, owner.ID, id, 3, "Permintaan pelanggan", "after-refund"); err != nil {
			t.Fatal(err)
		}
		replay, err := payments.Refund(ctx, owner.ID, refundInput(p, "refund-bank"))
		if err != nil || !replay.Replayed || replay.Refund.ID != r.Refund.ID {
			t.Fatal("closed replay", err)
		}
		if read(id).Order.Status() != domain.Cancelled {
			t.Fatal("cancelled state lost")
		}
	})
	t.Run("refund_versus_pickup_preserves_valid_net_balance", func(t *testing.T) {
		id := create("refund-pickup")
		p := pay(id, "paid", 10000, domain.Cash)
		if _, err := orders.ChangeStatus(ctx, cashier.ID, id, domain.Processing, 2, "processing"); err != nil {
			t.Fatal(err)
		}
		if _, err := orders.ChangeStatus(ctx, cashier.ID, id, domain.Ready, 3, "ready"); err != nil {
			t.Fatal(err)
		}
		errs := concurrentOrderCommands(2, func(i int) error {
			if i == 0 {
				_, err := payments.Refund(ctx, owner.ID, refundInput(p, "race-refund"))
				return err
			}
			_, err := orders.ChangeStatus(ctx, cashier.ID, id, domain.Collected, 4, "pickup")
			return err
		})
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatal("expected one winner", errs)
		}
		r := read(id)
		if errs[0] == nil {
			requireDomain(t, errs[1], domain.Conflict, "stale_order")
			if r.Order.Status() != domain.Ready || r.Order.Paid() != 0 {
				t.Fatal("refund winner invalid")
			}
		} else {
			requireDomain(t, errs[0], domain.Conflict, "order_closed")
			if r.Order.Status() != domain.Collected || r.Order.Paid() != 10000 {
				t.Fatal("pickup winner invalid")
			}
		}
	})
	t.Run("ledger_and_late_audit_failures_rollback_all_effects", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `INSERT INTO ledger_entries(id,type,amount,description,recorded_by,occurred_at)VALUES('deliberate-collision','KREDIT',1,'Contoh',$1,$2)`, owner.ID, fixed); err != nil {
			t.Fatal(err)
		}
		for _, stage := range []string{"ledger", "after-posting", "audit"} {
			id := create("rollback-" + stage)
			p := pay(id, "deposit", 3000, domain.Cash)
			in := refundInput(p, "retryable")
			_, err := NewPayments(refundFailureWork{base, stage}, rt).Refund(ctx, owner.ID, in)
			if err == nil {
				t.Fatal("injected failure not observed")
			}
			if r := read(id); r.Version != 2 || r.Order.Paid() != 3000 || r.Order.Payments()[0].Status != domain.PaymentPosted {
				t.Fatal("partial refund state survived")
			}
			if count(`SELECT count(*) FROM refunds WHERE order_id=$1`, id) != 0 || count(`SELECT count(*) FROM ledger_entries WHERE order_id=$1`, id) != 1 {
				t.Fatal("refund records survived rollback")
			}
			if _, err := payments.Refund(ctx, owner.ID, in); err != nil {
				t.Fatal("rolled back key poisoned", err)
			}
		}
	})
	t.Run("database_constraints_immutable_records_and_required_postings", func(t *testing.T) {
		id := create("constraints")
		p := pay(id, "deposit", 4000, domain.QRIS)
		assertCode := func(query, code string, args ...any) {
			t.Helper()
			_, err := db.ExecContext(ctx, query, args...)
			var pg *pq.Error
			if !errors.As(err, &pg) || string(pg.Code) != code {
				t.Fatalf("expected %s got %v", code, err)
			}
		}
		assertCode(`UPDATE payments SET amount=amount+1 WHERE id=$1`, "23514", p.ID)
		assertCode(`DELETE FROM payments WHERE id=$1`, "23514", p.ID)
		assertCode(`UPDATE ledger_entries SET amount=amount+1 WHERE order_id=$1`, "23514", id)
		assertCode(`INSERT INTO payments(id,order_id,idempotency_key,amount,method,recorded_by,received_at)VALUES('no-ledger',$1,'no-ledger',1,'cash',$2,$3)`, "23514", id, cashier.ID, rt.Clock.Now())
		assertCode(`INSERT INTO refunds(id,order_id,payment_id,idempotency_key,amount,method,recorded_by,refunded_at,reason)VALUES('partial',$1,$2,'partial',3999,'qris',$3,$4,'Koreksi')`, "23503", id, p.ID, owner.ID, rt.Clock.Now())
		assertCode(`INSERT INTO refunds(id,order_id,payment_id,idempotency_key,amount,method,recorded_by,refunded_at,reason)VALUES('wrong-method',$1,$2,'wrong-method',4000,'cash',$3,$4,'Koreksi')`, "23503", id, p.ID, owner.ID, rt.Clock.Now())
		assertCode(`INSERT INTO refunds(id,order_id,payment_id,idempotency_key,amount,method,recorded_by,refunded_at,reason)VALUES('no-refund-ledger',$1,$2,'no-ledger',4000,'qris',$3,$4,'Koreksi')`, "23514", id, p.ID, owner.ID, rt.Clock.Now())
		for _, kind := range []string{"payment", "refund"} {
			for _, mismatch := range []string{"method", "actor", "time"} {
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				func() {
					defer func() {
						if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
							t.Error(err)
						}
					}()
					method, actor, at := domain.QRIS, owner.ID, rt.Clock.Now()
					if mismatch == "method" {
						method = domain.BankTransfer
					}
					if mismatch == "actor" {
						actor = cashier.ID
					}
					if mismatch == "time" {
						at = at.Add(time.Second)
					}
					var query string
					if kind == "payment" {
						query = `INSERT INTO payments(id,order_id,idempotency_key,amount,method,recorded_by,received_at)VALUES('bad-posting',$1,'bad-posting',1,'qris',$2,$3)`
					} else {
						query = `INSERT INTO refunds(id,order_id,payment_id,idempotency_key,amount,method,recorded_by,refunded_at,reason)VALUES('bad-posting',$1,$4,'bad-posting',4000,'qris',$2,$3,'Koreksi')`
					}
					args := []any{id, owner.ID, rt.Clock.Now()}
					if kind == "refund" {
						args = append(args, p.ID)
					}
					if _, err := tx.ExecContext(ctx, query, args...); err != nil {
						t.Fatal(err)
					}
					query = `INSERT INTO ledger_entries(id,order_id,payment_id,type,amount,method,recorded_by,occurred_at,description)VALUES('bad-posting',$1,'bad-posting','DEBIT',1,$2,$3,$4,'Contoh')`
					if kind == "refund" {
						query = `INSERT INTO ledger_entries(id,order_id,refund_id,type,amount,method,recorded_by,occurred_at,description)VALUES('bad-posting',$1,'bad-posting','KREDIT',4000,$2,$3,$4,'Contoh')`
					}
					if _, err := tx.ExecContext(ctx, query, id, method, actor, at); err != nil {
						t.Fatal(err)
					}
					var pg *pq.Error
					if err := tx.Commit(); !errors.As(err, &pg) || string(pg.Code) != "23514" {
						t.Fatalf("mismatched %s %s posting committed: %v", kind, mismatch, err)
					}
				}()
			}
		}
		if count(`SELECT count(*) FROM refunds WHERE order_id=$1`, id) != 0 {
			t.Fatal("failed constraint left refund")
		}
		if _, err := payments.Refund(ctx, owner.ID, refundInput(p, "valid-refund")); err != nil {
			t.Fatal(err)
		}
		assertCode(`DELETE FROM refunds WHERE payment_id=$1`, "23514", p.ID)
		assertCode(`DELETE FROM ledger_entries WHERE order_id=$1`, "23514", id)
		if read(id).Order.Paid() != 0 {
			t.Fatal("constraint corrupted balance")
		}
	})
}
