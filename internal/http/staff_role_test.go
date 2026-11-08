package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
	"ulaundry/internal/testutil"
)

type roleWork struct {
	role   domain.Role
	denied []string
	writes int
}
type roleTx struct {
	repository.Tx
	work *roleWork
}

func (u *roleWork) View(ctx context.Context, fn func(repository.Readers) error) error {
	return fn(&roleTx{work: u})
}
func (u *roleWork) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	return fn(&roleTx{work: u})
}
func (u *roleWork) RecordAccessDenied(_ context.Context, _ string, permission string) error {
	u.denied = append(u.denied, permission)
	return nil
}
func (t *roleTx) LockStaffManagement(context.Context) error { return nil }
func (t *roleTx) GetStaff(context.Context, string) (domain.Staff, error) {
	at := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	return domain.Staff{ID: "actor-1", Username: "staff.test", Name: "Staff", Role: t.work.role, Active: true, CreatedAt: at, UpdatedAt: at}, nil
}
func (t *roleTx) ListStaff(ctx context.Context, _ repository.Page) ([]repository.StaffRecord, error) {
	v, err := t.GetStaff(ctx, "")
	return []repository.StaffRecord{{Staff: v, Version: 1}}, err
}
func (t *roleTx) AuditForEntity(context.Context, domain.AuditEntity, string, repository.Page) ([]domain.AuditEvent, error) {
	return []domain.AuditEvent{}, nil
}
func TestStaffHTTPPermissionMatrixUsesCurrentStaff(t *testing.T) {
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
		for _, tc := range []struct {
			method, path, body string
			want               int
		}{
			{"GET", "/api/staff", "", 200}, {"POST", "/api/staff", `{"username":"new.test","name":"New","role":"owner","password":"Frasa sandi sintetis"}`, 403}, {"PUT", "/api/staff/target/active", `{"active":false,"version":1}`, 403}, {"PUT", "/api/staff/target/role", `{"role":"owner","version":1}`, 403}, {"POST", "/api/staff/target/reset-password", `{"password":"Frasa sandi sintetis","version":1}`, 403}, {"GET", "/api/audit?entity=staff&entity_id=target", "", 200},
		} {
			if role == domain.Owner && tc.method != "GET" {
				continue
			}
			t.Run(string(role)+tc.method+tc.path, func(t *testing.T) {
				work := &roleWork{role: role}
				uow := service.WithAccessAudit(work, work)
				manager := testutil.TestSessions("synthetic-role-http-key-32bytes-only")
				// The signed claim says owner in every case; use-case permissions must use DB.
				token, err := manager.Tokens.Sign(domain.Staff{ID: "actor-1", Username: "staff.test", Role: domain.Owner}, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				claims, err := manager.Tokens.Parse(token, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				staff := service.NewStaffManagement(uow, service.Runtime{}, nil)
				h := handler.New(handler.Dependencies{Staff: staff})
				router := NewRouter(h, middleware.New([]string{manager.Origin}, manager), http.FS(fstest.MapFS{}))
				r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				r.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: token})
				r.Header.Set("Origin", manager.Origin)
				r.Header.Set("X-CSRF-Token", manager.CSRF(claims))
				if tc.body != "" {
					r.Header.Set("Content-Type", "application/json")
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				want := tc.want
				if strings.HasPrefix(tc.path, "/api/staff") && role != domain.Owner {
					want = 403
				}
				if strings.HasPrefix(tc.path, "/api/audit") && role == domain.Cashier {
					want = 403
				}
				if w.Code != want {
					t.Fatalf("status=%d want=%d", w.Code, want)
				}
				if want == 403 && (len(work.denied) != 1 || strings.Contains(w.Body.String(), "Frasa")) {
					t.Fatal("denial not audited/redacted")
				}
			})
		}
	}
}
