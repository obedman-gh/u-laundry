package migrate

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"
	"ulaundry/migrations"
)

func TestPostgresOrderWorkflowUpgrade(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	all, err := catalog(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
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
	for _, m := range all[:8] {
		if _, err = tx.ExecContext(ctx, m.sql); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum)VALUES($1,$2,$3)`, m.version, m.name, m.checksum); err != nil {
			t.Fatal(err)
		}
	}
	// Five valid historical states, with a zero-price snapshot. Schema 8 did not
	// record processing time, so upgrading must never invent that milestone.
	if _, err = tx.ExecContext(ctx, `
INSERT INTO staff(id,username,name,role,active,password_hash,created_at,updated_at)
VALUES('upgrade-owner','upgrade.test','Staf Contoh','owner',true,'unused-synthetic', '2026-11-01Z','2026-11-01Z');
INSERT INTO customers(id,name,contact,notes,created_at,updated_at)
VALUES('upgrade-customer','Pelanggan Contoh','','','2026-11-01Z','2026-11-01Z');
INSERT INTO services(id,name,unit,unit_price,turnaround_ns,active,created_at,updated_at)
VALUES('upgrade-service','Layanan Contoh','piece',0,3600000000000,true,'2026-11-01Z','2026-11-01Z');
INSERT INTO orders(id,customer_id,customer_name,customer_contact,customer_notes,customer_created_at,customer_updated_at,created_by,item_count,subtotal,discount,discount_reason,total,notes,status,created_at,updated_at,estimated_ready_at,ready_at,collected_at,cancelled_at,cancellation_reason,version)
SELECT 'upgrade-'||s,'upgrade-customer','Pelanggan Contoh','','','2026-11-01Z','2026-11-01Z','upgrade-owner',1,0,0,'',0,'',s,'2026-11-01Z','2026-11-02Z','2026-11-03Z',
CASE WHEN s IN('ready','collected') THEN '2026-11-01T01:00:00Z'::timestamptz END,
CASE WHEN s='collected' THEN '2026-11-02Z'::timestamptz END,
CASE WHEN s='cancelled' THEN '2026-11-02Z'::timestamptz END,
CASE WHEN s='cancelled' THEN 'Permintaan pelanggan' ELSE '' END,7
FROM unnest(ARRAY['received','processing','ready','collected','cancelled']) AS s;
INSERT INTO order_items(order_id,line_no,service_id,service_name,unit,unit_price,pieces,grams,line_total)
SELECT id,1,'upgrade-service','Layanan Contoh','piece',0,1,0,0 FROM orders;
`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	scalar := func(query string) string {
		t.Helper()
		var value string
		if err := db.QueryRowContext(ctx, query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	historyQuery := `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s WHERE version<=8`
	orderQuery := `SELECT json_agg(s ORDER BY id)::text FROM (SELECT id,status,version,total,created_at,updated_at,ready_at,collected_at,cancelled_at FROM orders) s`
	beforeHistory, beforeOrders := scalar(historyQuery), scalar(orderQuery)
	if err = Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	status, err := Status(ctx, db)
	if err != nil || len(status) != 10 || status[8].AppliedAt == nil {
		t.Fatal("version 9 not applied", err)
	}
	if err = Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	after, err := Status(ctx, db)
	if err != nil || !status[8].AppliedAt.Equal(*after[8].AppliedAt) {
		t.Fatal("repeat migration changed history", err)
	}
	if scalar(historyQuery) != beforeHistory || scalar(orderQuery) != beforeOrders {
		t.Fatal("upgrade changed existing history or order data")
	}
	store := postgres.New(db)
	if err = store.View(ctx, func(r repository.Readers) error {
		for _, state := range []domain.OrderStatus{domain.Received, domain.Processing, domain.Ready, domain.Collected, domain.Cancelled} {
			id := "upgrade-" + string(state)
			order, err := r.GetOrder(ctx, id)
			if err != nil {
				return err
			}
			if order.Order.Status() != state || order.Version != 7 || !order.Order.ProcessingAt().IsZero() {
				t.Fatal("historical state/milestone changed")
			}
			events, err := r.OrderHistory(ctx, id, repository.Page{})
			if err != nil {
				return err
			}
			if len(events) != 1 || events[0].Source != "baseline" || events[0].ActorID != "" || events[0].From != "" || events[0].To != state || events[0].Version != 7 || !events[0].OccurredAt.Equal(order.Order.UpdatedAt()) {
				t.Fatal("false historical transition", events)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if scalar(`SELECT count(*)::text FROM order_command_receipts`) != "0" {
		t.Fatal("upgrade invented command receipts")
	}
	// The updated writer can continue a historical processing order without
	// rewriting its unknown processing time or its baseline event.
	if err = store.WithinTransaction(ctx, func(tx repository.Tx) error {
		r, err := tx.LockOrder(ctx, "upgrade-processing")
		if err != nil {
			return err
		}
		at := r.Order.UpdatedAt().Add(time.Hour)
		if err = r.Order.Transition(domain.Ready, at, ""); err != nil {
			return err
		}
		if err = tx.SaveOrderProgress(ctx, r.Order, r.Version); err != nil {
			return err
		}
		return tx.AppendOrderStatusEvent(ctx, domain.OrderStatusEvent{OrderID: r.Order.ID(), ActorID: "upgrade-owner", Version: 8, From: domain.Processing, To: domain.Ready, OccurredAt: at, Source: "staff"})
	}); err != nil {
		t.Fatal(err)
	}
}
