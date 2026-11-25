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

func TestPostgresAuditUpgrade(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
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
	if _, err := tx.ExecContext(ctx, `CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT UNIQUE NOT NULL,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	for _, m := range all[:11] {
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum)VALUES($1,$2,$3)`, m.version, m.name, m.checksum); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO staff(id,username,name,role,active,password_hash,created_at,updated_at)VALUES('owner','owner.test','Staf Contoh','owner',true,'synthetic-unused','2026-11-01Z','2026-11-01Z');
 INSERT INTO audit_events(id,actor_id,action,entity,entity_id,occurred_at)VALUES('historical','owner','staff_logged_in','staff','owner','2026-11-01Z');
 INSERT INTO auth_events(kind,actor_id,request_id,occurred_at)VALUES('login_failure',NULL,'legacy-request','2026-11-01Z');`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	scalar := func(query string) string {
		t.Helper()
		var s string
		if err := db.QueryRowContext(ctx, query).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	history := `SELECT json_agg(s ORDER BY version)::text FROM schema_migrations s WHERE version<=11`
	old := `SELECT row_to_json(s)::text FROM(SELECT id,actor_id,action,entity,entity_id,occurred_at FROM audit_events WHERE id='historical')s`
	auth := `SELECT json_agg(s ORDER BY id)::text FROM auth_events s`
	beforeHistory, beforeOld, beforeAuth := scalar(history), scalar(old), scalar(auth)
	if err := Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	if scalar(history) != beforeHistory || scalar(old) != beforeOld || scalar(auth) != beforeAuth || scalar(`SELECT request_id IS NULL AND metadata='{}'::jsonb FROM audit_events WHERE id='historical'`) != "true" {
		t.Fatal("history changed or fabricated correlation")
	}
	for _, query := range []string{`UPDATE audit_events SET action='staff_updated'`, `DELETE FROM audit_events`, `TRUNCATE audit_events`, `UPDATE auth_events SET kind='logout'`, `DELETE FROM auth_events`, `TRUNCATE auth_events`,
		`INSERT INTO audit_events(id,actor_id,action,entity,entity_id,occurred_at)VALUES('missing-request','owner','staff_created','staff','owner',now())`,
		`INSERT INTO audit_events(id,actor_id,action,entity,entity_id,occurred_at,request_id,metadata)VALUES('unsafe','owner','staff_created','staff','owner',now(),'request-1','{"password":"secret"}')`,
		`INSERT INTO audit_events(id,actor_id,action,entity,entity_id,occurred_at,request_id,metadata)VALUES('bad-type','owner','staff_created','staff','owner',now(),'request-1','{"role":"private text"}')`,
		`INSERT INTO audit_events(id,actor_id,action,entity,entity_id,occurred_at,request_id,metadata)VALUES('bad-number','owner','staff_created','staff','owner',now(),'request-1','{"version":-1}')`,
		`INSERT INTO audit_events(id,actor_id,action,entity,entity_id,occurred_at,request_id,metadata)VALUES('bad-shape','owner','staff_created','staff','owner',now(),'request-1','[]')`,
	} {
		if _, err := db.ExecContext(ctx, query); err == nil {
			t.Fatal("unsafe audit write accepted", query)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_events(id,actor_id,action,entity,entity_id,occurred_at,request_id,metadata)VALUES('valid','owner','credentials_verified','staff','owner',now(),'server-request','{"version":1,"role":"owner","active":true,"unit_price":0,"status":"received"}')`); err != nil {
		t.Fatal(err)
	}
}
