package migrate

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
	"ulaundry/internal/testutil"
	"ulaundry/migrations"
)

func TestPostgresShiftUpgrade(t *testing.T) {
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
	for _, m := range all[:10] {
		if _, err = tx.ExecContext(ctx, m.sql); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum)VALUES($1,$2,$3)`, m.version, m.name, m.checksum); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO staff(id,username,name,role,active,password_hash,created_at,updated_at)VALUES('owner','owner','Staf Contoh','owner',true,'unused-synthetic','2026-11-01Z','2026-11-01Z');
INSERT INTO ledger_entries(id,type,amount,description,recorded_by,occurred_at,method)VALUES('historical','DEBIT',500,'Kas historis','owner','2026-11-01Z','cash');
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
	history := `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s WHERE version<=10`
	ledger := `SELECT row_to_json(s)::text FROM(SELECT id,type,amount,description,recorded_by,occurred_at,method FROM ledger_entries WHERE id='historical')s`
	beforeHistory, beforeLedger := scalar(history), scalar(ledger)
	if err = Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	states, err := Status(ctx, db)
	if err != nil || len(states) != 11 || states[10].AppliedAt == nil {
		t.Fatal("shift migration pending", err)
	}
	if err = Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	after, err := Status(ctx, db)
	if err != nil || !states[10].AppliedAt.Equal(*after[10].AppliedAt) {
		t.Fatal("repeat upgrade rewrote history", err)
	}
	if scalar(history) != beforeHistory || scalar(ledger) != beforeLedger || scalar(`SELECT shift_id IS NULL FROM ledger_entries WHERE id='historical'`) != "true" || scalar(`SELECT count(*)::text FROM cash_shifts`) != "0" {
		t.Fatal("upgrade changed money or invented shifts")
	}
}
