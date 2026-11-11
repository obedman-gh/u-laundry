package migrate

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"ulaundry/internal/config"
	"ulaundry/internal/platform"
	"ulaundry/migrations"
)

// This suite never reads .env or DB_* from the developer's environment.
// It requires an explicitly supplied admin DSN, creates random test databases,
// and only drops those databases. No tables in the admin database are mutated.
func TestPostgresMigrations(t *testing.T) {
	dsn := os.Getenv("ULAUNDRY_TEST_ADMIN_DSN")
	if dsn == "" {
		t.Skip("set ULAUNDRY_TEST_ADMIN_DSN to a dedicated PostgreSQL test cluster")
	}
	settings, err := config.ParseDatabase(func(key string) string {
		switch key {
		case "APP_ENV":
			return "test"
		case "DB_DSN":
			return dsn
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Name != "postgres" {
		t.Fatal("test admin DSN must target postgres in a dedicated test cluster")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := platform.OpenDatabase(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	newDB := func(t *testing.T, demo ...bool) *sql.DB {
		t.Helper()
		suffix := make([]byte, 8)
		if _, err := rand.Read(suffix); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("ulaundry_migration_test_%x", suffix)
		if len(demo) > 0 && demo[0] {
			name += "_demo"
		}
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+pq.QuoteIdentifier(name)+` TEMPLATE template0`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			if _, err := admin.ExecContext(cleanupCtx, `DROP DATABASE `+pq.QuoteIdentifier(name)); err != nil {
				t.Error(err)
			}
		})
		dbSettings := settings
		dbSettings.Name = name
		db, err := platform.OpenDatabase(ctx, dbSettings)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
		return db
	}
	exec := func(t *testing.T, db *sql.DB, query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	scalar := func(t *testing.T, db *sql.DB, query string) string {
		t.Helper()
		var value string
		if err := db.QueryRowContext(ctx, query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	assertApplied := func(t *testing.T, db *sql.DB) {
		t.Helper()
		states, err := Status(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if len(states) != 9 {
			t.Fatal(states)
		}
		for _, state := range states {
			if state.AppliedAt == nil {
				t.Fatal("pending migration", state.Name)
			}
		}
	}
	expectViolation := func(t *testing.T, db *sql.DB, query, code string) {
		t.Helper()
		_, err := db.ExecContext(ctx, query)
		var pg *pq.Error
		if !errors.As(err, &pg) || string(pg.Code) != code {
			t.Fatalf("expected %s, got %v", code, err)
		}
	}
	legacy := func(t *testing.T, db *sql.DB) {
		t.Helper()
		body, err := os.ReadFile("testdata/legacy.sql")
		if err != nil {
			t.Fatal(err)
		}
		exec(t, db, string(body))
		exec(t, db, `INSERT INTO pesanan VALUES ('OLD-1','Customer test','Cuci',12000,'Diproses','2025-01-02 10:11:12',NULL,NULL);
            INSERT INTO kasir (username,password,nama_lengkap) VALUES ('staff-test','synthetic-hash','Staff test');
            INSERT INTO keuangan (tanggal,tipe,keterangan,nominal) VALUES ('2025-01-02 10:12:13','DEBIT','Pembayaran OLD-1',12000);`)
	}
	snapshots := []string{
		`SELECT row_to_json(x)::text FROM (SELECT id_pesanan,nama_customer,layanan,total_biaya,status,waktu_pesan,waktu_perkiraan_selesai,waktu_selesai_real FROM pesanan WHERE id_pesanan='OLD-1') x`,
		`SELECT row_to_json(x)::text FROM (SELECT id_kasir,username,password,nama_lengkap FROM kasir WHERE username='staff-test') x`,
		`SELECT row_to_json(x)::text FROM (SELECT id_transaksi,tanggal,tipe,keterangan,nominal FROM keuangan WHERE id_transaksi=1) x`,
	}

	t.Run("fresh_constraints_timestamps_indexes_and_repeat", func(t *testing.T) {
		db := newDB(t)
		states, err := Status(ctx, db)
		if err != nil || len(states) != 9 || states[0].AppliedAt != nil || states[1].AppliedAt != nil || states[2].AppliedAt != nil || states[3].AppliedAt != nil {
			t.Fatalf("fresh status = %v, %v", states, err)
		}
		if scalar(t, db, `SELECT to_regclass('public.schema_migrations') IS NULL`) != "true" {
			t.Fatal("status mutated schema")
		}
		if err := Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		assertApplied(t, db)
		if scalar(t, db, `SELECT (SELECT count(*) FROM pesanan)+(SELECT count(*) FROM kasir)+(SELECT count(*) FROM keuangan)`) != "0" {
			t.Fatal("schema seeded data")
		}
		historyBefore := scalar(t, db, `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s`)
		if err := Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		if scalar(t, db, `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s`) != historyBefore {
			t.Fatal("repeat up changed history")
		}
		exec(t, db, `INSERT INTO pesanan (id_pesanan,nama_customer,layanan,total_biaya,status) VALUES ('TEST','Test','Cuci',0,'Diproses');
            INSERT INTO kasir (username,password,nama_lengkap) VALUES ('test','hash','Test');
            INSERT INTO keuangan (tipe,keterangan,nominal,id_pesanan) VALUES ('DEBIT','Test',0,'TEST');
            INSERT INTO keuangan (tipe,keterangan,nominal) VALUES ('KREDIT','Unlinked expense',0);`)
		cases := []struct{ query, code string }{
			{`UPDATE pesanan SET total_biaya=-1`, "23514"},
			{`UPDATE keuangan SET nominal=-1`, "23514"},
			{`UPDATE keuangan SET tipe='OTHER'`, "23514"},
			{`INSERT INTO kasir (username,password,nama_lengkap) VALUES ('test','hash','Duplicate')`, "23505"},
			{`UPDATE kasir SET username=NULL`, "23502"},
			{`UPDATE pesanan SET waktu_pesan=NULL`, "23502"},
			{`UPDATE keuangan SET tanggal=NULL`, "23502"},
			{`UPDATE keuangan SET id_pesanan='MISSING'`, "23503"},
			{`DELETE FROM pesanan WHERE id_pesanan='TEST'`, "23503"},
			{`UPDATE pesanan SET id_pesanan='CHANGED' WHERE id_pesanan='TEST'`, "23503"},
		}
		for _, c := range cases {
			expectViolation(t, db, c.query, c.code)
		}
		for _, table := range []string{"pesanan", "kasir", "keuangan"} {
			exec(t, db, `UPDATE `+table+` SET updated_at='2000-01-01'`)
			if scalar(t, db, `SELECT bool_and(updated_at > '2000-01-02'::timestamptz) FROM `+table) != "true" {
				t.Fatal("timestamp trigger missing", table)
			}
			expectViolation(t, db, `INSERT INTO `+table+` DEFAULT VALUES`, "23502")
		}
		if scalar(t, db, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname IN ('keuangan_tipe_nominal_idx','keuangan_id_pesanan_idx')`) != "2" {
			t.Fatal("indexes missing")
		}
	})

	t.Run("partial_legacy_install", func(t *testing.T) {
		db := newDB(t)
		legacy(t, db)
		exec(t, db, `DROP TABLE pesanan, kasir`)
		if err := Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		assertApplied(t, db)
		if scalar(t, db, `SELECT count(*) FROM keuangan WHERE nominal=12000`) != "1" {
			t.Fatal("partial legacy ledger lost")
		}
	})

	t.Run("reject_float_money_without_conversion", func(t *testing.T) {
		db := newDB(t)
		legacy(t, db)
		exec(t, db, `ALTER TABLE keuangan ALTER COLUMN nominal TYPE double precision`)
		if err := Up(ctx, db); err == nil {
			t.Fatal("floating monetary column accepted")
		}
		if scalar(t, db, `SELECT nominal FROM keuangan WHERE id_transaksi=1`) != "12000" {
			t.Fatal("legacy value changed")
		}
		if scalar(t, db, `SELECT to_regclass('public.schema_migrations') IS NULL`) != "true" {
			t.Fatal("failed adoption recorded")
		}
	})

	t.Run("demo_opt_in_empty_database_only", func(t *testing.T) {
		body, err := os.ReadFile("../../scripts/demo.sql")
		if err != nil {
			t.Fatal(err)
		}
		// A dedicated connection allows explicit rollback after the guarded SQL
		// script fails inside BEGIN, matching psql closing a failed session.
		applyDemo := func(db *sql.DB) error {
			conn, err := db.Conn(ctx)
			if err != nil {
				return err
			}
			defer func() {
				if err := conn.Close(); err != nil {
					t.Error(err)
				}
			}()
			_, err = conn.ExecContext(ctx, string(body))
			if err != nil {
				if _, rollbackErr := conn.ExecContext(ctx, `ROLLBACK`); rollbackErr != nil {
					t.Error(rollbackErr)
				}
			}
			return err
		}
		db := newDB(t, true)
		if err := Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		if err := applyDemo(db); err != nil {
			t.Fatal(err)
		}
		if err := applyDemo(db); err == nil {
			t.Fatal("demo repeated on populated database")
		}
		if scalar(t, db, `SELECT (SELECT count(*) FROM pesanan)+(SELECT count(*) FROM keuangan)`) != "2" {
			t.Fatal("demo duplicate or rollback lost data")
		}
		if scalar(t, db, `SELECT count(*) FROM kasir`) != "0" {
			t.Fatal("demo created account")
		}
		other := newDB(t)
		if err := Up(ctx, other); err != nil {
			t.Fatal(err)
		}
		if err := applyDemo(other); err == nil {
			t.Fatal("demo accepted non-demo database name")
		}
		if scalar(t, other, `SELECT count(*) FROM pesanan`) != "0" {
			t.Fatal("demo mutated non-demo database")
		}
		domainData := newDB(t, true)
		if err := Up(ctx, domainData); err != nil {
			t.Fatal(err)
		}
		exec(t, domainData, `INSERT INTO public.customers (id,name,created_at,updated_at) VALUES ('test','Test',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
		if err := applyDemo(domainData); err == nil {
			t.Fatal("demo accepted populated domain tables")
		}
		if scalar(t, domainData, `SELECT count(*) FROM pesanan`) != "0" {
			t.Fatal("demo changed legacy data alongside domain data")
		}
	})

	t.Run("upgrade_preserves_legacy_values_and_sequences", func(t *testing.T) {
		db := newDB(t)
		legacy(t, db)
		before := make([]string, len(snapshots))
		for i, query := range snapshots {
			before[i] = scalar(t, db, query)
		}
		if err := Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		assertApplied(t, db)
		for i, query := range snapshots {
			if scalar(t, db, query) != before[i] {
				t.Fatal("legacy row changed", i)
			}
		}
		if scalar(t, db, `SELECT id_pesanan IS NULL FROM keuangan WHERE id_transaksi=1`) != "true" {
			t.Fatal("guessed ledger association")
		}
		if scalar(t, db, `INSERT INTO kasir (username,password,nama_lengkap) VALUES ('next','hash','Next') RETURNING id_kasir`) != "2" {
			t.Fatal("sequence changed")
		}
		if scalar(t, db, `INSERT INTO keuangan (tipe,keterangan,nominal) VALUES ('KREDIT','Next',1) RETURNING id_transaksi`) != "2" {
			t.Fatal("ledger sequence changed")
		}
	})

	t.Run("older_schema_and_pending_version", func(t *testing.T) {
		db := newDB(t)
		legacy(t, db)
		exec(t, db, `ALTER TABLE pesanan DROP COLUMN waktu_pesan, DROP COLUMN waktu_perkiraan_selesai, DROP COLUMN waktu_selesai_real`)
		// Simulate a genuine prior release which had only version 1 applied.
		all, err := catalog(migrations.Files)
		if err != nil {
			t.Fatal(err)
		}
		exec(t, db, all[0].sql)
		exec(t, db, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY,name TEXT NOT NULL UNIQUE,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`)
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version,name,checksum) VALUES ($1,$2,$3)`, all[0].version, all[0].name, all[0].checksum); err != nil {
			t.Fatal(err)
		}
		states, err := Status(ctx, db)
		if err != nil || states[0].AppliedAt == nil || states[1].AppliedAt != nil {
			t.Fatalf("pending status = %v, %v", states, err)
		}
		priorHistory := scalar(t, db, `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s`)
		exec(t, db, `UPDATE pesanan SET total_biaya=-1`)
		if err := Up(ctx, db); err == nil {
			t.Fatal("invalid pending version accepted")
		}
		if scalar(t, db, `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s`) != priorHistory {
			t.Fatal("failed pending migration altered committed history")
		}
		if scalar(t, db, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND column_name='updated_at'`) != "0" {
			t.Fatal("failed pending migration retained DDL")
		}
		exec(t, db, `UPDATE pesanan SET total_biaya=12000`)
		if err := Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		assertApplied(t, db)
		if scalar(t, db, `SELECT count(*) FROM pesanan WHERE waktu_pesan IS NOT NULL`) != "1" {
			t.Fatal("timestamp adoption failed")
		}
	})

	for _, c := range []struct{ name, invalid, repair string }{
		{"negative_order", `UPDATE pesanan SET total_biaya=-1`, `UPDATE pesanan SET total_biaya=12000`},
		{"negative_ledger", `UPDATE keuangan SET nominal=-1`, `UPDATE keuangan SET nominal=12000`},
		{"unknown_ledger_type", `UPDATE keuangan SET tipe='OTHER'`, `UPDATE keuangan SET tipe='DEBIT'`},
		{"null_order_timestamp", `UPDATE pesanan SET waktu_pesan=NULL`, `UPDATE pesanan SET waktu_pesan='2025-01-02 10:11:12'`},
		{"null_ledger_timestamp", `UPDATE keuangan SET tanggal=NULL`, `UPDATE keuangan SET tanggal='2025-01-02 10:12:13'`},
		{"duplicate_username", `ALTER TABLE kasir DROP CONSTRAINT kasir_username_key; INSERT INTO kasir (username,password,nama_lengkap) VALUES ('staff-test','hash','Duplicate')`, `UPDATE kasir SET username='other-test' WHERE id_kasir=2`},
	} {
		t.Run("invalid_upgrade_"+c.name, func(t *testing.T) {
			db := newDB(t)
			legacy(t, db)
			exec(t, db, c.invalid)
			before := make([]string, len(snapshots))
			// Username duplicate changes snapshot cardinality; use stable ID for it.
			queries := append([]string(nil), snapshots...)
			queries[1] = strings.ReplaceAll(queries[1], "username='staff-test'", "id_kasir=1")
			for i, query := range queries {
				before[i] = scalar(t, db, query)
			}
			if err := Up(ctx, db); err == nil {
				t.Fatal("invalid upgrade accepted")
			}
			if scalar(t, db, `SELECT to_regclass('public.schema_migrations') IS NULL`) != "true" {
				t.Fatal("history survived rollback")
			}
			if scalar(t, db, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND column_name='updated_at'`) != "0" {
				t.Fatal("DDL survived rollback")
			}
			for i, query := range queries {
				if scalar(t, db, query) != before[i] {
					t.Fatal("invalid user data changed")
				}
			}
			// Only test-owned synthetic data is repaired here, never by the runner.
			exec(t, db, c.repair)
			if err := Up(ctx, db); err != nil {
				t.Fatal(err)
			}
			assertApplied(t, db)
		})
	}

	t.Run("lock_contention_and_cancellation_release", func(t *testing.T) {
		db := newDB(t)
		holder, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := holder.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Error(err)
			}
		})
		if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
			t.Fatal(err)
		}
		if err := Up(ctx, db); err == nil || !strings.Contains(err.Error(), "sedang berjalan") {
			t.Fatalf("concurrent runner: %v", err)
		}
		if err := holder.Rollback(); err != nil {
			t.Fatal(err)
		}
		// A relation lock blocks DDL after the advisory lock has been acquired.
		legacy(t, db)
		blocker, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := blocker.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Error(err)
			}
		})
		if _, err := blocker.ExecContext(ctx, `LOCK TABLE pesanan IN ACCESS EXCLUSIVE MODE`); err != nil {
			t.Fatal(err)
		}
		canceledCtx, done := context.WithTimeout(ctx, 150*time.Millisecond)
		err = Up(canceledCtx, db)
		done()
		if err == nil {
			t.Fatal("blocked migration ignored timeout")
		}
		if err := blocker.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err := Up(ctx, db); err != nil {
			t.Fatalf("advisory lock not released after cancel: %v", err)
		}
		assertApplied(t, db)
	})

	t.Run("upgrade_version_two_preserves_committed_history_and_legacy", func(t *testing.T) {
		db := newDB(t)
		legacy(t, db)
		all, err := catalog(migrations.Files)
		if err != nil {
			t.Fatal(err)
		}
		exec(t, db, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY,name TEXT NOT NULL UNIQUE,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`)
		for _, m := range all[:2] {
			exec(t, db, m.sql)
			if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES ($1,$2,$3)`, m.version, m.name, m.checksum); err != nil {
				t.Fatal(err)
			}
		}
		beforeHistory := scalar(t, db, `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s`)
		before := make([]string, len(snapshots))
		for i, q := range snapshots {
			before[i] = scalar(t, db, q)
		}
		if err := Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		assertApplied(t, db)
		if scalar(t, db, `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s WHERE version<=2`) != beforeHistory {
			t.Fatal("committed history changed")
		}
		for i, q := range snapshots {
			if scalar(t, db, q) != before[i] {
				t.Fatal("legacy changed during additive upgrade")
			}
		}
		if scalar(t, db, `SELECT count(*) FROM public.orders`) != "0" {
			t.Fatal("automatic legacy conversion occurred")
		}
	})

	for _, mutation := range []string{
		`UPDATE schema_migrations SET checksum=repeat('0',64) WHERE version=1`,
		`DELETE FROM schema_migrations WHERE version=1`,
		`UPDATE schema_migrations SET name='changed' WHERE version=1`,
		`INSERT INTO schema_migrations (version,name,checksum) VALUES (7,'future',repeat('0',64))`,
	} {
		t.Run("history_drift", func(t *testing.T) {
			db := newDB(t)
			if err := Up(ctx, db); err != nil {
				t.Fatal(err)
			}
			exec(t, db, mutation)
			if _, err := Status(ctx, db); err == nil {
				t.Fatal("status accepted drift")
			}
			if err := Up(ctx, db); err == nil {
				t.Fatal("up accepted drift")
			}
		})
	}
}
