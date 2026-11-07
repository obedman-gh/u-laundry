package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"
	"ulaundry/migrations"
)

type auditFailureUow struct {
	base repository.UnitOfWork
	fail bool
}

func (u *auditFailureUow) View(ctx context.Context, fn func(repository.Readers) error) error {
	return u.base.View(ctx, fn)
}
func (u *auditFailureUow) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	return u.base.WithinTransaction(ctx, func(tx repository.Tx) error { return fn(auditFailureTx{Tx: tx, fail: u.fail}) })
}

type auditFailureTx struct {
	repository.Tx
	fail bool
}

func (t auditFailureTx) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	if t.fail && e.Action == domain.PaymentRecorded {
		return failure
	}
	return t.Tx.AppendAudit(ctx, e)
}

func TestPostgresPaymentUseCaseAtomicity(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	// Reproduce the committed version-3 release, then verify the actual upgrade
	// to version 4 without touching a user database or rewriting old history.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	}()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE public.schema_migrations(version INTEGER PRIMARY KEY,name TEXT UNIQUE NOT NULL,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range names[:3] {
		body, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, i+1, name, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO public.pesanan(id_pesanan,nama_customer,layanan,total_biaya,status) VALUES ('LEGACY-1','Synthetic legacy','Cuci',10000,'Diproses')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	before, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	after, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 5 || after[3].AppliedAt == nil {
		t.Fatal("workflow migration missing")
	}
	for i := 0; i < 3; i++ {
		if before[i].AppliedAt == nil || !before[i].AppliedAt.Equal(*after[i].AppliedAt) {
			t.Fatal("old history changed")
		}
	}
	var legacy int64
	if err := db.QueryRowContext(ctx, `SELECT total_biaya FROM public.pesanan WHERE id_pesanan='LEGACY-1'`).Scan(&legacy); err != nil || legacy != 10000 {
		t.Fatal("legacy data changed", err)
	}

	base := postgres.New(db)
	hash, err := bcrypt.GenerateFromPassword([]byte("synthetic-only"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	actor := domain.Staff{ID: "actor-1", Username: "staff.test", Name: "Staf Contoh", Role: domain.Admin, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	if err := base.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddStaff(ctx, actor, string(hash)) }); err != nil {
		t.Fatal(err)
	}
	_, rt := fixture(t, domain.Admin)
	c, err := NewCustomers(base, rt).Create(ctx, actor.ID, CustomerInput{Name: "Pelanggan Test"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewCatalog(base, rt).Configure(ctx, actor.ID, ServiceInput{Name: "Cuci", Unit: domain.PerKilogram, UnitPrice: 6000, Turnaround: 24 * time.Hour, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	v, err = NewCatalog(base, rt).Configure(ctx, actor.ID, ServiceInput{ID: v.Service.ID, ExpectedVersion: v.Version, Name: "Cuci", Unit: domain.PerKilogram, UnitPrice: 7000, Turnaround: 24 * time.Hour, Active: true})
	if err != nil || v.Version != 2 {
		t.Fatal("catalog update/version", err)
	}
	o, err := NewOrders(base, rt).Create(ctx, actor.ID, OrderInput{CustomerID: c.ID, Items: []domain.ItemRequest{{ServiceID: v.Service.ID, Quantity: domain.Quantity{Grams: 2500}}}, Discount: 500, DiscountReason: "Penyesuaian"})
	if err != nil || o.Order.Total() != 17000 {
		t.Fatal("order calculation", err)
	}
	var storedHash []byte
	if err := db.QueryRowContext(ctx, `SELECT token_hash FROM public.order_tracking_tokens WHERE order_id=$1`, o.Order.ID()).Scan(&storedHash); err != nil || len(storedHash) != 32 || string(storedHash) == o.TrackingToken {
		t.Fatal("raw tracking token stored", err)
	}
	failing := &auditFailureUow{base: base, fail: true}
	command := PaymentInput{OrderID: o.Order.ID(), IdempotencyKey: "payment-key-1", Amount: 10000, Method: domain.Cash}
	result, err := NewPayments(failing, rt).Record(ctx, actor.ID, command)
	if !errors.Is(err, failure) || result.Payment.ID != "" {
		t.Fatal("audit failure lost or payment leaked", err)
	}
	if err := base.View(ctx, func(r repository.Readers) error {
		stored, err := r.GetOrder(ctx, o.Order.ID())
		if err != nil {
			return err
		}
		if stored.Order.Paid() != 0 || stored.Version != 1 {
			t.Fatal("payment survived rollback")
		}
		ledger, err := r.LedgerForOrder(ctx, o.Order.ID(), repository.Page{})
		if err != nil {
			return err
		}
		if len(ledger) != 0 {
			t.Fatal("ledger survived rollback")
		}
		_, err = r.FindPaymentByKey(ctx, o.Order.ID(), command.IdempotencyKey)
		if !errors.Is(err, repository.ErrNotFound) {
			t.Fatal("payment key survived rollback", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	failing.fail = false
	pay := NewPayments(failing, rt)
	result, err = pay.Record(ctx, actor.ID, command)
	if err != nil || result.Replayed {
		t.Fatal(err)
	}
	replayed, err := pay.Record(ctx, actor.ID, command)
	if err != nil || !replayed.Replayed || replayed.Payment.ID != result.Payment.ID {
		t.Fatal("replay unstable", err)
	}
	if err := base.View(ctx, func(r repository.Readers) error {
		stored, err := r.GetOrder(ctx, o.Order.ID())
		if err != nil {
			return err
		}
		if stored.Order.Paid() != 10000 || stored.Version != 2 || stored.Order.Status() != domain.Received {
			t.Fatal("money/status/version mismatch")
		}
		events, err := r.AuditForEntity(ctx, domain.PaymentEntity, result.Payment.ID, repository.Page{})
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].Action != domain.PaymentRecorded {
			t.Fatal("duplicate/missing payment audit")
		}
		items, err := r.ListOrders(ctx, repository.OrderFilter{Status: domain.Received})
		if err != nil {
			return err
		}
		if len(items) != 1 || len(items[0].ServiceNames) != 1 || items[0].ServiceNames[0] != "Cuci" {
			t.Fatal("list projection not persisted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	view, err := NewTracking(base).Get(ctx, o.TrackingToken)
	if err != nil || view.Status != domain.Received {
		t.Fatal("tracking projection", err)
	}
	report, err := NewReports(base, rt).Generate(ctx, actor.ID, ReportPeriod{})
	if err != nil || report.Inflow != 10000 || report.Outflow != 0 || report.Balance != 10000 {
		t.Fatal("ledger report", err)
	}
}
