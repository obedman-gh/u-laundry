package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"
	"ulaundry/migrations"
)

type catalogHoldWork struct {
	repository.UnitOfWork
	acquired, release chan struct{}
}
type catalogHoldTx struct {
	repository.Tx
	work *catalogHoldWork
}

func (u *catalogHoldWork) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	return u.UnitOfWork.WithinTransaction(ctx, func(tx repository.Tx) error { return fn(catalogHoldTx{tx, u}) })
}
func (tx catalogHoldTx) LockService(ctx context.Context, id string) (repository.ServiceRecord, error) {
	r, err := tx.Tx.LockService(ctx, id)
	if err != nil {
		return r, err
	}
	close(tx.work.acquired)
	select {
	case <-tx.work.release:
		return r, nil
	case <-ctx.Done():
		return repository.ServiceRecord{}, ctx.Err()
	}
}

func TestPostgresCatalogHistoryAndOrderSnapshots(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Reproduce schema 7, including a catalog row at version 3 and an accepted order.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	}()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT UNIQUE NOT NULL,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range names[:7] {
		body, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum)VALUES($1,$2,$3)`, i+1, name, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO services(id,name,unit,unit_price,turnaround_ns,active,created_at,updated_at,version)VALUES('wash','Cuci Lama','kg',7005,$1,true,$2,$2,3)`, int64(24*time.Hour), fixed); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	base := postgres.New(db)
	actor := domain.Staff{ID: "actor", Username: "staff.test", Name: "Synthetic Owner", Role: domain.Owner, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	customer, err := domain.NewCustomer("customer", "Synthetic Customer", "", "", fixed)
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("unused-synthetic"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	svc := domain.Service{ID: "wash", Name: "Cuci Lama", Unit: domain.PerKilogram, UnitPrice: 7005, Turnaround: 24 * time.Hour, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	old, err := domain.NewOrder(domain.NewOrderInput{ID: "order-before-upgrade", CreatedAt: fixed, Items: []domain.ItemRequest{{ServiceID: svc.ID, Quantity: domain.Quantity{Grams: 100}}}, Discount: 1, DiscountReason: "Penyesuaian"}, customer, []domain.Service{svc}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = base.WithinTransaction(ctx, func(tx repository.Tx) error {
		if err := tx.AddStaff(ctx, actor, string(passwordHash)); err != nil {
			return err
		}
		if err := tx.AddCustomer(ctx, customer); err != nil {
			return err
		}
		return tx.CreateOrder(ctx, old)
	}); err != nil {
		t.Fatal(err)
	}
	before, err := migrate.Status(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	after, err := migrate.Status(ctx, db)
	if err != nil || len(after) != 8 || after[7].AppliedAt == nil {
		t.Fatal("schema 8", err)
	}
	for i := 0; i < 7; i++ {
		if !before[i].AppliedAt.Equal(*after[i].AppliedAt) {
			t.Fatal("old migration history changed")
		}
	}
	rt := Runtime{Clock: ClockFunc(func() time.Time { return fixed.Add(time.Hour) }), IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	catalog := NewCatalog(base, rt)
	orders := NewOrders(base, rt)
	history, err := catalog.History(ctx, actor.ID, svc.ID, repository.Page{})
	if err != nil || len(history) != 1 || history[0].Version != 3 || history[0].Source != "baseline" || history[0].ActorID != "" {
		t.Fatal("baseline", err)
	}
	config := ServiceInput{ID: svc.ID, Name: "Cuci Baru", Unit: domain.PerKilogram, UnitPrice: 8000, Turnaround: 48 * time.Hour, Active: true, DisplayOrder: 20, ExpectedVersion: 3}
	updated, err := catalog.Configure(ctx, actor.ID, config)
	if err != nil || updated.Version != 4 {
		t.Fatal(err)
	}
	in := OrderInput{CustomerID: customer.ID, Items: []domain.ItemRequest{{ServiceID: svc.ID, Quantity: domain.Quantity{Grams: 100}}}, Discount: 1, DiscountReason: "Penyesuaian"}
	newOrder, err := orders.Create(ctx, actor.ID, in)
	if err != nil || newOrder.Order.Total() != 799 || !newOrder.Order.EstimatedReadyAt().Equal(rt.Clock.Now().Add(48*time.Hour)) {
		t.Fatal("new price/duration", err)
	}
	stored, err := orders.Get(ctx, actor.ID, old.ID())
	if err != nil || stored.Order.Total() != 700 || stored.Order.Subtotal() != 701 || stored.Order.Discount().Amount != 1 || stored.Order.Items()[0].ServiceName != "Cuci Lama" || stored.Order.Items()[0].UnitPrice != 7005 || stored.Order.Items()[0].Quantity.Grams != 100 {
		t.Fatal("old snapshot changed", err)
	}
	config.ExpectedVersion = 3
	_, err = catalog.Configure(ctx, actor.ID, config)
	requireDomain(t, err, domain.Conflict, "")
	config.ExpectedVersion = 4
	config.UnitPrice = 9999
	_, err = NewCatalog(privacyAuditFailure{base}, rt).Configure(ctx, actor.ID, config)
	if !errors.Is(err, failure) {
		t.Fatal("audit failure", err)
	}
	current, err := catalog.Get(ctx, actor.ID, svc.ID)
	if err != nil || current.Version != 4 || current.Service.UnitPrice != 8000 {
		t.Fatal("partial price update", err)
	}
	history, err = catalog.History(ctx, actor.ID, svc.ID, repository.Page{})
	if err != nil || len(history) != 2 || history[0].Version != 4 || history[0].ActorID != actor.ID || history[0].Service.UnitPrice != 8000 || history[0].Service.DisplayOrder != 20 {
		t.Fatal("revision rollback/roundtrip", err)
	}
	// Confirm that an order holding the catalog lock commits before a price editor.
	hold := &catalogHoldWork{UnitOfWork: base, acquired: make(chan struct{}), release: make(chan struct{})}
	orderDone := make(chan error, 1)
	go func() { _, e := NewOrders(hold, rt).Create(ctx, actor.ID, in); orderDone <- e }()
	select {
	case <-hold.acquired:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	config.UnitPrice = 9000
	editDone := make(chan error, 1)
	go func() { _, e := catalog.Configure(ctx, actor.ID, config); editDone <- e }()
	deadline := time.Now().Add(5 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(hold.release)
	if !blocked {
		t.Fatal("price editor did not wait for order snapshot lock")
	}
	if err = <-orderDone; err != nil {
		t.Fatal(err)
	}
	if err = <-editDone; err != nil {
		t.Fatal(err)
	}
	postEdit, err := orders.Create(ctx, actor.ID, in)
	if err != nil || postEdit.Order.Total() != 899 {
		t.Fatal("committed new price", err)
	}
	config.ExpectedVersion = 5
	config.Unit = domain.PerPiece
	config.UnitPrice = 20000
	if _, err = catalog.Configure(ctx, actor.ID, config); err != nil {
		t.Fatal(err)
	}
	in.Items[0].Quantity = domain.Quantity{Pieces: 2}
	in.Discount = 0
	in.DiscountReason = ""
	pieceOrder, err := orders.Create(ctx, actor.ID, in)
	if err != nil || pieceOrder.Order.Total() != 40000 || pieceOrder.Order.Items()[0].Unit != domain.PerPiece {
		t.Fatal("new unit", err)
	}
	stored, err = orders.Get(ctx, actor.ID, old.ID())
	if err != nil || stored.Order.Items()[0].Unit != domain.PerKilogram || stored.Order.Total() != 700 {
		t.Fatal("old unit rewritten", err)
	}
	config.ExpectedVersion = 6
	config.Active = false
	if _, err = catalog.Configure(ctx, actor.ID, config); err != nil {
		t.Fatal(err)
	}
	_, err = orders.Create(ctx, actor.ID, in)
	requireDomain(t, err, domain.Conflict, "service_inactive")
	active, err := catalog.List(ctx, actor.ID, repository.ServiceFilter{})
	if err != nil || len(active) != 0 {
		t.Fatal("inactive listed", err)
	}
	all, err := catalog.List(ctx, actor.ID, repository.ServiceFilter{IncludeInactive: true})
	if err != nil || len(all) != 1 || all[0].Service.Active {
		t.Fatal("management inactive list", err)
	}
	history, err = catalog.History(ctx, actor.ID, svc.ID, repository.Page{Limit: 2, Offset: 1})
	if err != nil || len(history) != 2 || history[0].Version != 6 || history[1].Version != 5 {
		t.Fatal("history pagination", err)
	}
	// Constraint failures are separate implicit transactions and cannot alter data.
	for _, statement := range []string{
		`UPDATE services SET display_order=-1 WHERE id='wash'`,
		`UPDATE services SET display_order=1000001 WHERE id='wash'`,
		`UPDATE services SET unit_price=-1 WHERE id='wash'`,
		`UPDATE services SET unit='litre' WHERE id='wash'`,
		`UPDATE services SET turnaround_ns=0 WHERE id='wash'`,
		`UPDATE order_items SET line_total=line_total+1 WHERE order_id='order-before-upgrade'`,
		`UPDATE order_items SET pieces=1 WHERE order_id='order-before-upgrade'`,
		`UPDATE orders SET total=total+1 WHERE id='order-before-upgrade'`,
		`UPDATE service_revisions SET unit_price=1 WHERE service_id='wash'`,
		`DELETE FROM service_revisions WHERE service_id='wash'`,
	} {
		_, err := db.ExecContext(ctx, statement)
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "23514" {
			t.Fatal("constraint/immutability", statement, err)
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO service_revisions SELECT * FROM service_revisions WHERE service_id='wash' AND version=7`)
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatal("revision uniqueness", err)
	}
	for _, name := range []string{"Zulu", "Alpha"} {
		_, err = catalog.Configure(ctx, actor.ID, ServiceInput{Name: name, Unit: domain.PerPiece, UnitPrice: 1, Turnaround: time.Hour, Active: true, DisplayOrder: 10})
		if err != nil {
			t.Fatal(err)
		}
	}
	all, err = catalog.List(ctx, actor.ID, repository.ServiceFilter{IncludeInactive: true})
	if err != nil || len(all) != 3 || all[0].Service.Name != "Alpha" || all[1].Service.Name != "Zulu" || all[2].Service.ID != "wash" {
		t.Fatal("display ordering", err)
	}
}

func TestPostgresCatalogDemo(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source")
	}
	script, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "scripts", "catalog-demo.sql"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t.Run("reject_non_demo_database", func(t *testing.T) {
		db := testutil.Postgres(t)
		if err := migrate.Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(script)); err == nil {
			t.Fatal("unguarded seed")
		}
	})
	t.Run("synthetic_catalog_and_repeat_guard", func(t *testing.T) {
		db := testutil.PostgresDemo(t)
		if err := migrate.Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(script)); err != nil {
			t.Fatal(err)
		}
		var services, revisions, pii int
		if err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM services),(SELECT count(*) FROM service_revisions WHERE source='demo'),(SELECT count(*) FROM customers)+(SELECT count(*) FROM staff)+(SELECT count(*) FROM orders)+(SELECT count(*) FROM ledger_entries)`).Scan(&services, &revisions, &pii); err != nil || services != 3 || revisions != 3 || pii != 0 {
			t.Fatal("demo contents", err)
		}
		if _, err := db.ExecContext(ctx, string(script)); err == nil {
			t.Fatal("repeat seed changed existing catalog")
		}
	})
}
