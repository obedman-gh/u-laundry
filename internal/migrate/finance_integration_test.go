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

func TestPostgresFinanceUpgrade(t *testing.T) {
	for _, fault := range []string{"", "missing_posting", "posting_time", "invalid_key"} {
		name := fault
		if name == "" {
			name = "preserves_receipts_and_backfills_methods"
		}
		t.Run(name, func(t *testing.T) {
			db := testutil.Postgres(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
			for _, m := range all[:9] {
				if _, err = tx.ExecContext(ctx, m.sql); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum)VALUES($1,$2,$3)`, m.version, m.name, m.checksum); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = tx.ExecContext(ctx, `
INSERT INTO staff(id,username,name,role,active,password_hash,created_at,updated_at)
VALUES('owner','upgrade.owner','Staf Contoh','owner',true,'unused-synthetic','2026-11-01Z','2026-11-01Z');
INSERT INTO customers(id,name,contact,notes,created_at,updated_at)
VALUES('customer','Pelanggan Contoh','','','2026-11-01Z','2026-11-01Z');
INSERT INTO services(id,name,unit,unit_price,turnaround_ns,active,created_at,updated_at)
VALUES('service','Layanan Contoh','piece',10000,3600000000000,true,'2026-11-01Z','2026-11-01Z');
INSERT INTO orders(id,customer_id,customer_name,customer_contact,customer_notes,customer_created_at,customer_updated_at,created_by,item_count,subtotal,discount,discount_reason,total,notes,status,created_at,updated_at,estimated_ready_at)
VALUES('order','customer','Pelanggan Contoh','','','2026-11-01Z','2026-11-01Z','owner',1,10000,0,'',10000,'','received','2026-11-01Z','2026-11-01Z','2026-11-02Z');
INSERT INTO order_items(order_id,line_no,service_id,service_name,unit,unit_price,pieces,grams,line_total)
VALUES('order',1,'service','Layanan Contoh','piece',10000,1,0,10000);
INSERT INTO payments(id,order_id,idempotency_key,amount,method,recorded_by,received_at) VALUES
('cash','order','cash-key',3000,'cash','owner','2026-11-01T01:00:00Z'),
('qris','order','qris-key',2000,'qris','owner','2026-11-01T01:00:00Z');
INSERT INTO ledger_entries(id,order_id,payment_id,type,amount,description,recorded_by,occurred_at)
SELECT 'ledger-'||id,order_id,id,'DEBIT',amount,'Penerimaan contoh',recorded_by,received_at FROM payments;
INSERT INTO ledger_entries(id,type,amount,description,recorded_by,occurred_at)
VALUES('expense','KREDIT',500,'Pengeluaran contoh','owner','2026-11-01T01:00:00Z');
UPDATE orders SET version=version+1,updated_at='2026-11-01T01:00:00Z' WHERE id='order';
`); err != nil {
				t.Fatal(err)
			}
			faultSQL := map[string]string{
				"missing_posting": `DELETE FROM ledger_entries WHERE payment_id='cash'`,
				"posting_time":    `UPDATE ledger_entries SET occurred_at=occurred_at+interval '1 second' WHERE payment_id='cash'`,
				"invalid_key":     `UPDATE payments SET idempotency_key='bad/key' WHERE id='cash'`,
			}
			if query := faultSQL[fault]; query != "" {
				if _, err = tx.ExecContext(ctx, query); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			scalar := func(query string) string {
				t.Helper()
				var v string
				if err := db.QueryRowContext(ctx, query).Scan(&v); err != nil {
					t.Fatal(err)
				}
				return v
			}
			historyQuery := `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s WHERE version<=9`
			paymentQuery := `SELECT json_agg(s ORDER BY id)::text FROM (SELECT id,order_id,idempotency_key,amount,method,recorded_by,received_at FROM payments) s`
			ledgerQuery := `SELECT json_agg(s ORDER BY id)::text FROM (SELECT id,order_id,payment_id,type,amount,description,recorded_by,occurred_at FROM ledger_entries) s`
			orderQuery := `SELECT row_to_json(s)::text FROM orders s WHERE id='order'`
			beforeHistory, beforePayments, beforeLedger, beforeOrder := scalar(historyQuery), scalar(paymentQuery), scalar(ledgerQuery), scalar(orderQuery)
			err = Up(ctx, db)
			if fault != "" {
				if err == nil || scalar(`SELECT count(*)::text FROM schema_migrations`) != "9" || scalar(`SELECT to_regclass('public.refunds') IS NULL`) != "true" || scalar(`SELECT count(*)::text FROM information_schema.columns WHERE table_schema='public' AND table_name='payments' AND column_name='status'`) != "0" {
					t.Fatal("unsafe upgrade was accepted or partially applied", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				states, err := Status(ctx, db)
				if err != nil || len(states) != 11 || states[9].AppliedAt == nil {
					t.Fatal("schema 10 pending", err)
				}
				if err = Up(ctx, db); err != nil {
					t.Fatal(err)
				}
				after, err := Status(ctx, db)
				if err != nil || !states[9].AppliedAt.Equal(*after[9].AppliedAt) {
					t.Fatal("repeat upgrade rewrote history", err)
				}
				if scalar(`SELECT count(*)::text FROM payments WHERE status='posted'`) != "2" || scalar(`SELECT count(*)::text FROM refunds`) != "0" || scalar(`SELECT count(*)::text FROM ledger_entries l JOIN payments p ON p.id=l.payment_id WHERE l.method=p.method`) != "2" || scalar(`SELECT method IS NULL FROM ledger_entries WHERE id='expense'`) != "true" {
					t.Fatal("upgrade invented reversals or changed methods")
				}
				if err = postgres.New(db).View(ctx, func(r repository.Readers) error {
					o, err := r.GetOrder(ctx, "order")
					if err != nil {
						return err
					}
					if o.Order.Paid() != 5000 || o.Order.Balance() != 5000 || o.Order.PaymentStatus() != domain.PartiallyPaid {
						t.Fatal("upgrade changed balance")
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if scalar(historyQuery) != beforeHistory || scalar(paymentQuery) != beforePayments || scalar(ledgerQuery) != beforeLedger || scalar(orderQuery) != beforeOrder {
				t.Fatal("upgrade changed historical financial data")
			}
		})
	}
}
