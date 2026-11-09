package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sync"
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

type privacyAuditFailure struct{ repository.UnitOfWork }
type privacyAuditTx struct{ repository.Tx }

func (u privacyAuditFailure) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	return u.UnitOfWork.WithinTransaction(ctx, func(tx repository.Tx) error { return fn(privacyAuditTx{tx}) })
}
func (t privacyAuditTx) AppendAudit(context.Context, domain.AuditEvent) error { return failure }

func TestPostgresCustomerPrivacy(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Exercise the actual 6 -> 7 upgrade with a pre-existing customer and token.
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
	for i, name := range names[:6] {
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
	if _, err = tx.ExecContext(ctx, `INSERT INTO customers(id,name,contact,notes,created_at,updated_at)VALUES('existing','Synthetic 50%_Customer','','',now(),now());
INSERT INTO staff(id,username,name,role,active,password_hash,created_at,updated_at)VALUES('existing-staff','existing.test','Synthetic Staff','cashier',true,'not-used-for-login',now(),now());
INSERT INTO orders(id,customer_id,customer_name,customer_contact,customer_notes,customer_created_at,customer_updated_at,created_by,item_count,subtotal,discount,discount_reason,total,notes,status,created_at,updated_at,estimated_ready_at)VALUES('existing-order','existing','Synthetic 50%_Customer','','',now(),now(),'existing-staff',1,1000,0,'',1000,'','received',now(),now(),now()+interval '1 day');
INSERT INTO order_tracking_tokens(order_id,token_hash)VALUES('existing-order',decode(repeat('ab',32),'hex'));`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
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
	if err != nil || len(after) != 8 || after[6].AppliedAt == nil {
		t.Fatal("upgrade", err)
	}
	for i := 0; i < 6; i++ {
		if !before[i].AppliedAt.Equal(*after[i].AppliedAt) {
			t.Fatal("migration history changed")
		}
	}
	var grace time.Time
	if err = db.QueryRowContext(ctx, `SELECT expires_at FROM order_tracking_tokens WHERE order_id='existing-order'`).Scan(&grace); err != nil || time.Until(grace) < 89*24*time.Hour || time.Until(grace) > 91*24*time.Hour {
		t.Fatal("old token grace", err)
	}
	base := postgres.New(db)
	rt := Runtime{Clock: platform.SystemClock{}, IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	customers := NewCustomers(base, rt)
	orders := NewOrders(base, rt)
	tracking := NewTracking(base, rt)
	c, err := customers.Create(ctx, "existing-staff", CustomerInput{Name: "Synthetic name only"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Contact != "" || c.Address != "" || c.Notes != "" {
		t.Fatal("extra personal data required")
	}
	updated, err := customers.Update(ctx, "existing-staff", c.ID, CustomerInput{Name: "Synthetic updated", Contact: "081234567890", Address: "Viola", Notes: "Synthetic notes"}, 1)
	if err != nil || updated.Version != 2 {
		t.Fatal(err)
	}
	_, err = customers.Update(ctx, "existing-staff", c.ID, CustomerInput{Name: "stale"}, 1)
	requireDomain(t, err, domain.Conflict, "")
	_, err = NewCustomers(privacyAuditFailure{base}, rt).Update(ctx, "existing-staff", c.ID, CustomerInput{Name: "rollback"}, 2)
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	stored, err := customers.Get(ctx, "existing-staff", c.ID)
	if err != nil || stored.Customer.Name != updated.Customer.Name || stored.Version != 2 || stored.Customer.Address != "Viola" {
		t.Fatal("customer rollback/round trip", err)
	}
	for _, q := range []string{"50%_", "0812-3456-7890"} {
		found, err := customers.Search(ctx, "existing-staff", q, repository.Page{})
		if err != nil || len(found) != 1 {
			t.Fatal("literal/contact search", q, err)
		}
	}
	if err = base.View(ctx, func(r repository.Readers) error {
		all, err := r.SearchCustomers(ctx, "", repository.Page{Limit: 1})
		if err == nil && len(all) != 1 {
			t.Fatal("pagination")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	v, err := NewCatalog(base, rt).Configure(ctx, "existing-staff", ServiceInput{Name: "Denied", Unit: domain.PerPiece, UnitPrice: 1000, Turnaround: time.Hour, Active: true})
	_ = v
	requireDomain(t, err, domain.Forbidden, "")
	// Add catalog through the repository so no admin account is needed.
	at := time.Now().UTC().Truncate(time.Microsecond)
	svc := domain.Service{ID: "wash", Name: "Cuci", Unit: domain.PerPiece, UnitPrice: 1000, Turnaround: time.Hour, Active: true, CreatedAt: at, UpdatedAt: at}
	if err = base.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddService(ctx, svc) }); err != nil {
		t.Fatal(err)
	}
	in := OrderInput{CustomerID: c.ID, Items: []domain.ItemRequest{{ServiceID: svc.ID, Quantity: domain.Quantity{Pieces: 1}}}}
	o, err := orders.Create(ctx, "existing-staff", in)
	if err != nil {
		t.Fatal(err)
	}
	view, err := tracking.Get(ctx, o.TrackingToken)
	if err != nil || view.Status != domain.Received || view.Code == o.Order.ID() || view.Message == "" {
		t.Fatal("public projection", err)
	}
	hash, _ := domain.TrackingHash(o.TrackingToken)
	if err = base.View(ctx, func(r repository.Readers) error {
		_, err := r.GetTracking(ctx, hash, o.TrackingExpiresAt)
		if !errors.Is(err, repository.ErrNotFound) {
			t.Fatal("expiry boundary", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"ORDER-1", "", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		_, err := tracking.Get(ctx, token)
		requireDomain(t, err, domain.NotFound, "")
	}
	// Same raw token on another order fails UNIQUE and rolls back order + audit.
	duplicate := rt
	duplicate.Tokens = GeneratorFunc(func() (string, error) { return o.TrackingToken, nil })
	bad, err := NewOrders(base, duplicate).Create(ctx, "existing-staff", in)
	if !errors.Is(err, repository.ErrConflict) || bad.TrackingToken != "" {
		t.Fatal("duplicate capability accepted", err)
	}
	history, err := orders.List(ctx, "existing-staff", repository.OrderFilter{CustomerID: c.ID})
	if err != nil || len(history) != 1 {
		t.Fatal("failed create leaked into history", err)
	}
	// A failed audit keeps the old link and order version intact.
	badRotation, err := NewOrders(privacyAuditFailure{base}, rt).RotateTracking(ctx, "existing-staff", o.Order.ID(), 1)
	if !errors.Is(err, failure) || badRotation.Token != "" {
		t.Fatal("rotation audit rollback", err)
	}
	if _, err = tracking.Get(ctx, o.TrackingToken); err != nil {
		t.Fatal("old link revoked on rollback", err)
	}
	_, err = NewOrders(base, duplicate).RotateTracking(ctx, "existing-staff", o.Order.ID(), 1)
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatal("rotation reused current token", err)
	}
	var wg sync.WaitGroup
	results := make(chan struct {
		value RotatedTracking
		err   error
	}, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := orders.RotateTracking(ctx, "existing-staff", o.Order.ID(), 1)
			results <- struct {
				value RotatedTracking
				err   error
			}{v, e}
		}()
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	var active RotatedTracking
	for result := range results {
		if result.err == nil {
			success++
			active = result.value
		} else {
			requireDomain(t, result.err, domain.Conflict, "")
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 || active.Version != 2 {
		t.Fatal("rotation race")
	}
	if _, err = tracking.Get(ctx, o.TrackingToken); err == nil {
		t.Fatal("revoked token still works")
	}
	if _, err = tracking.Get(ctx, active.Token); err != nil {
		t.Fatal("replacement unavailable", err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE order_tracking_tokens SET expires_at=now()-interval '1 second' WHERE order_id=$1`, o.Order.ID()); err != nil {
		t.Fatal(err)
	}
	_, err = tracking.Get(ctx, active.Token)
	requireDomain(t, err, domain.NotFound, "")
	if _, err = db.ExecContext(ctx, `UPDATE order_tracking_tokens SET expires_at='infinity' WHERE order_id=$1`, o.Order.ID()); err == nil {
		t.Fatal("infinite expiry accepted")
	}
	// Updating the live customer must preserve the accepted order snapshot.
	_, err = customers.Update(ctx, "existing-staff", c.ID, CustomerInput{Name: "Later name"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	r, err := orders.Get(ctx, "existing-staff", o.Order.ID())
	if err != nil || r.Order.Customer().Name != "Synthetic updated" || r.Version != 2 {
		t.Fatal("order snapshot/version", err)
	}
	history, err = orders.List(ctx, "existing-staff", repository.OrderFilter{CustomerID: c.ID})
	if err != nil || len(history) != 1 {
		t.Fatal("customer history", err)
	}
}
