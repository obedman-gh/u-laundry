package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/security"
	"ulaundry/internal/testutil"
	"ulaundry/migrations"
)

func TestPostgresStaffAdministration(t *testing.T) {
	t.Run("bootstrap_serialized_and_closed", func(t *testing.T) {
		db := testutil.Postgres(t)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		if err := migrate.Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		store := postgres.New(db)
		rt := Runtime{Clock: platform.SystemClock{}, IDs: platform.RandomIDs{}}
		s := NewStaffManagement(WithAccessAudit(store, store), rt, platform.Passwords{})
		results := make(chan error, 2)
		var group sync.WaitGroup
		for i := 0; i < 2; i++ {
			group.Add(1)
			go func(i int) {
				defer group.Done()
				_, err := s.Bootstrap(ctx, StaffInput{Username: fmt.Sprintf("owner-%d", i), Name: "Synthetic Owner", Password: "Frasa sandi sintetis"})
				results <- err
			}(i)
		}
		group.Wait()
		close(results)
		success, closed := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else {
				var e *domain.Error
				if !errors.As(err, &e) || e.Code != "bootstrap_closed" {
					t.Fatal(err)
				}
				closed++
			}
		}
		var owners, audits int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM staff WHERE role='owner' AND active`).Scan(&owners); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action='staff_bootstrapped'`).Scan(&audits); err != nil {
			t.Fatal(err)
		}
		if success != 1 || closed != 1 || owners != 1 || audits != 1 {
			t.Fatal("bootstrap concurrency bypass")
		}
	})
	t.Run("upgrade_roles_constraints_audit_and_revocation", func(t *testing.T) {
		db := testutil.Postgres(t)
		ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
		defer cancel()
		// Reproduce version 5 before the additive upgrade, including an existing user.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		rollback := func() {
			if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
				t.Error(e)
			}
		}
		defer rollback()
		if _, err = tx.ExecContext(ctx, `CREATE TABLE public.schema_migrations(version INTEGER PRIMARY KEY,name TEXT UNIQUE NOT NULL,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp())`); err != nil {
			t.Fatal(err)
		}
		names, err := fs.Glob(migrations.Files, "*.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		for i, name := range names[:5] {
			body, err := fs.ReadFile(migrations.Files, name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.ExecContext(ctx, string(body)); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, i+1, name, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
				t.Fatal(err)
			}
		}
		hash, err := (platform.Passwords{}).Hash("Frasa sandi owner sintetis")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO staff(id,username,name,role,active,password_hash,created_at,updated_at) VALUES('owner-existing','owner.test','Synthetic Owner','owner',true,$1,now(),now())`, hash); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if err = migrate.Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		if err = migrate.Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		var preserved string
		var version int64
		if err = db.QueryRowContext(ctx, `SELECT password_hash,version FROM staff WHERE id='owner-existing' AND role='owner' AND active`).Scan(&preserved, &version); err != nil || preserved != hash || version != 1 {
			t.Fatal("upgrade changed existing identity", err)
		}
		// One connection proves denial events are written after the rejected view/tx.
		db.SetMaxOpenConns(1)
		store := postgres.New(db)
		uow := WithAccessAudit(store, store)
		rt := Runtime{Clock: platform.SystemClock{}, IDs: platform.RandomIDs{}}
		s := NewStaffManagement(uow, rt, platform.Passwords{})
		_, err = s.Bootstrap(ctx, StaffInput{Username: "another.owner", Name: "No", Password: "Frasa sandi sintetis"})
		requireDomain(t, err, domain.Conflict, "bootstrap_closed")
		created, err := s.Create(ctx, "owner-existing", StaffInput{Username: "cashier.test", Name: "Synthetic Cashier", Role: domain.Cashier, Password: "Frasa sandi kasir sintetis"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Create(ctx, "owner-existing", StaffInput{Username: "cashier.test", Name: "Duplicate", Role: domain.Admin, Password: "Frasa sandi kasir sintetis"})
		if !errors.Is(err, repository.ErrConflict) {
			t.Fatal("username uniqueness missing", err)
		}
		if _, err = db.ExecContext(ctx, `UPDATE staff SET role='invalid' WHERE id=$1`, created.Staff.ID); err == nil {
			t.Fatal("DB role constraint missing")
		}
		_, err = s.List(ctx, created.Staff.ID, repository.Page{})
		requireDomain(t, err, domain.Forbidden, "forbidden")
		_, err = NewCatalog(uow, rt).Configure(ctx, created.Staff.ID, ServiceInput{})
		requireDomain(t, err, domain.Forbidden, "forbidden")
		_, err = NewReports(uow, rt).Generate(ctx, created.Staff.ID, ReportPeriod{})
		requireDomain(t, err, domain.Forbidden, "forbidden")
		_, err = s.Audit(ctx, created.Staff.ID, domain.StaffEntity, created.Staff.ID, repository.Page{})
		requireDomain(t, err, domain.Forbidden, "forbidden")
		var denied int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM auth_events WHERE kind='access_denied' AND actor_id=$1`, created.Staff.ID).Scan(&denied); err != nil || denied != 4 {
			t.Fatal("denial audit lost", err)
		}
		manager := &security.Manager{Tokens: security.Tokens{Secret: "synthetic-role-session-key-32bytes", Issuer: "https://fixture.invalid", Audience: "ulaundry-staff", Lifetime: 8 * time.Hour}, Store: store, Secret: "synthetic-cookie-session-key-32bytes", Origin: "https://fixture.invalid"}
		openSession := func(v repository.StaffRecord) *http.Request {
			t.Helper()
			var hash string
			if err := db.QueryRowContext(ctx, `SELECT password_hash FROM staff WHERE id=$1`, v.Staff.ID).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			token, err := manager.Tokens.Sign(v.Staff, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.Start(ctx, token, sha256.Sum256([]byte(hash)), "synthetic-request"); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", manager.Origin+"/api/session", nil)
			r.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: token})
			return r
		}
		old := openSession(created)
		updated, err := s.Change(ctx, "owner-existing", created.Staff.ID, "role", StaffChange{Version: 1, Role: domain.Admin})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = manager.Authenticate(old); err == nil {
			t.Fatal("role change retained session")
		}
		_, err = s.List(ctx, updated.Staff.ID, repository.Page{})
		requireDomain(t, err, domain.Forbidden, "forbidden")
		if _, err = s.Audit(ctx, updated.Staff.ID, domain.StaffEntity, updated.Staff.ID, repository.Page{}); err != nil {
			t.Fatal("admin audit permission missing", err)
		}
		old = openSession(updated)
		updated, err = s.Change(ctx, "owner-existing", updated.Staff.ID, "password", StaffChange{Version: 2, Password: "Frasa sandi pengganti sintetis"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = manager.Authenticate(old); err == nil {
			t.Fatal("password reset retained session")
		}
		var newHash string
		if err = db.QueryRowContext(ctx, `SELECT password_hash FROM staff WHERE id=$1`, updated.Staff.ID).Scan(&newHash); err != nil || bcrypt.CompareHashAndPassword([]byte(newHash), []byte("Frasa sandi pengganti sintetis")) != nil {
			t.Fatal("reset hash incorrect", err)
		}
		old = openSession(updated)
		if _, err = db.ExecContext(ctx, `CREATE FUNCTION public.test_reject_staff_change() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='staff_deactivated' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_staff_change BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION public.test_reject_staff_change()`); err != nil {
			t.Fatal(err)
		}
		_, err = s.Change(ctx, "owner-existing", updated.Staff.ID, "active", StaffChange{Version: 3, Active: false})
		if err == nil {
			t.Fatal("audit failure returned success")
		}
		if _, err = manager.Authenticate(old); err != nil {
			t.Fatal("rolled-back change revoked session", err)
		}
		if _, err = db.ExecContext(ctx, `DROP TRIGGER test_reject_staff_change ON audit_events`); err != nil {
			t.Fatal(err)
		}
		updated, err = s.Change(ctx, "owner-existing", updated.Staff.ID, "active", StaffChange{Version: 3, Active: false})
		if err != nil || updated.Staff.Active {
			t.Fatal("deactivate failed", err)
		}
		if _, err = manager.Authenticate(old); err == nil {
			t.Fatal("deactivated user retained session")
		}
		_, err = s.Change(ctx, "owner-existing", updated.Staff.ID, "role", StaffChange{Version: 1, Role: domain.Owner})
		requireDomain(t, err, domain.Conflict, "stale_staff")
		_, err = s.Change(ctx, "owner-existing", "owner-existing", "active", StaffChange{Version: 1, Active: false})
		requireDomain(t, err, domain.Conflict, "self_lockout")
		rows, err := s.List(ctx, "owner-existing", repository.Page{})
		if err != nil {
			t.Fatal(err)
		}
		serialized, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(serialized), hash) || strings.Contains(string(serialized), newHash) || strings.Contains(string(serialized), "Password") {
			t.Fatal("staff read leaked credential")
		}
	})
}
