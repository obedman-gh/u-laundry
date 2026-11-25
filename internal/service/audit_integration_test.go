package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/security"
	"ulaundry/internal/testutil"
)

func TestPostgresAuditTrail(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(security.WithCorrelationID(context.Background(), "server-audit-request"), 75*time.Second)
	defer cancel()
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := postgres.New(db)
	uow := WithAccessAudit(store, store)
	now := fixed
	rt := Runtime{Clock: ClockFunc(func() time.Time { return now }), IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	staff := NewStaffManagement(uow, rt, platform.Passwords{})
	password := "Frasa uji audit sintetis 2026"
	owner, err := staff.Bootstrap(ctx, StaffInput{Username: "audit.owner", Name: "Owner Contoh", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	cashier, err := staff.Create(ctx, owner.Staff.ID, StaffInput{Username: "audit.cashier", Name: "Kasir Contoh", Password: password, Role: domain.Cashier})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := staff.Create(ctx, owner.Staff.ID, StaffInput{Username: "audit.admin", Name: "Admin Contoh", Password: password, Role: domain.Admin})
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []struct {
		op     string
		change StaffChange
	}{{"role", StaffChange{Version: 1, Role: domain.Admin}}, {"role", StaffChange{Version: 2, Role: domain.Cashier}}, {"password", StaffChange{Version: 3, Password: "Frasa pengganti audit sintetis"}}, {"active", StaffChange{Version: 4, Active: false}}, {"active", StaffChange{Version: 5, Active: true}}} {
		if _, err := staff.Change(ctx, owner.Staff.ID, cashier.Staff.ID, in.op, in.change); err != nil {
			t.Fatal(err)
		}
	}
	catalog := NewCatalog(uow, rt)
	svc, err := catalog.Configure(ctx, owner.Staff.ID, ServiceInput{Name: "Layanan Contoh", Unit: domain.PerPiece, UnitPrice: 10000, Turnaround: time.Hour, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	svc, err = catalog.Configure(ctx, owner.Staff.ID, ServiceInput{ID: svc.Service.ID, ExpectedVersion: 1, Name: "Layanan Contoh", Unit: domain.PerPiece, UnitPrice: 12000, Turnaround: time.Hour, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCustomers(uow, rt).Create(ctx, cashier.Staff.ID, CustomerInput{Name: "Nama Privat", Contact: "081234567890", Notes: "Catatan privat"})
	if err != nil {
		t.Fatal(err)
	}
	orders := NewOrders(uow, rt)
	payments := NewPayments(uow, rt)
	shifts := NewShifts(uow, rt)
	create := func(key string) string {
		t.Helper()
		v, err := orders.Create(ctx, cashier.Staff.ID, OrderInput{IdempotencyKey: key, CustomerID: c.ID, Items: []domain.ItemRequest{{ServiceID: svc.Service.ID, Quantity: domain.Quantity{Pieces: 1}}}})
		if err != nil {
			t.Fatal(err)
		}
		return v.Order.ID()
	}
	id := create("success-order")
	o, err := orders.Get(ctx, owner.Staff.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.ChangeStatus(ctx, cashier.Staff.ID, id, domain.Processing, o.Version, "processing"); err != nil {
		t.Fatal(err)
	}
	shift, err := shifts.Open(ctx, owner.Staff.ID, ShiftOpenInput{Key: "shift-open", Opening: 0})
	if err != nil {
		t.Fatal(err)
	}
	p, err := payments.Record(ctx, owner.Staff.ID, PaymentInput{OrderID: id, Amount: 12000, Method: domain.Cash, IdempotencyKey: "payment"})
	if err != nil {
		t.Fatal(err)
	}
	refund, err := payments.Refund(ctx, owner.Staff.ID, RefundInput{OrderID: id, PaymentID: p.Payment.ID, IdempotencyKey: "refund", Reason: "Alasan privat"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shifts.Close(ctx, owner.Staff.ID, ShiftCloseInput{ID: shift.Shift.ID, Key: "close", Physical: 0}); err != nil {
		t.Fatal(err)
	}
	tokens := security.Tokens{Secret: "synthetic-audit-jwt-signing-key-32bytes", Issuer: "https://fixture.invalid", Audience: "ulaundry-staff", Lifetime: time.Hour}
	login, err := NewAuth(uow, rt, platform.Passwords{}, tokens).Login(ctx, owner.Staff.Username, password)
	if err != nil {
		t.Fatal(err)
	}
	manager := &security.Manager{Tokens: tokens, Store: store, Now: func() time.Time { return now }}
	claims, err := manager.Start(ctx, login.Token, login.CredentialFingerprint, "server-audit-request")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Logout(ctx, claims, "server-audit-request"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Audit(ctx, "login_failure", "server-audit-request"); err != nil {
		t.Fatal(err)
	}
	trail := NewAuditTrail(uow, rt)
	f := repository.AuditFilter{From: fixed.Add(-time.Hour), To: fixed.Add(time.Hour), Page: repository.Page{Limit: 100}}
	history, err := trail.List(ctx, owner.Staff.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range history.Page.Items {
		seen[e.Action] = true
		if e.RequestID != "server-audit-request" {
			t.Fatal("correlation lost", e)
		}
		if e.Action == "service_configured" && (e.Metadata.UnitPrice == nil || e.Metadata.Version < 1) {
			t.Fatal("price metadata missing")
		}
	}
	for _, action := range []string{"staff_bootstrapped", "staff_created", "staff_role_changed", "staff_password_reset", "staff_deactivated", "staff_activated", "service_configured", "order_created", "order_status_changed", "payment_recorded", "payment_refunded", "shift_closed", "credentials_verified", "login_success", "login_failure", "logout"} {
		if !seen[action] {
			t.Fatal("missing sensitive action", action)
		}
	}
	raw, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{password, "Frasa pengganti audit sintetis", "Nama Privat", "081234567890", "Catatan privat", "Alasan privat", login.Token, "Layanan Contoh"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("audit leaked private payload")
		}
	}
	for _, actor := range []string{admin.Staff.ID, cashier.Staff.ID} {
		_, err := trail.List(ctx, actor, f)
		requireDomain(t, err, domain.Forbidden, "")
		_, err = staff.Audit(ctx, actor, domain.ShiftEntity, shift.Shift.ID, repository.Page{})
		requireDomain(t, err, domain.Forbidden, "")
	}
	for _, entity := range []struct {
		kind domain.AuditEntity
		id   string
	}{{domain.ShiftEntity, shift.Shift.ID}, {domain.RefundEntity, refund.Refund.ID}} {
		v, err := staff.Audit(ctx, owner.Staff.ID, entity.kind, entity.id, repository.Page{})
		if err != nil || len(v) == 0 {
			t.Fatal("entity history", entity, v, err)
		}
	}
	t.Run("date-action-page-filters", func(t *testing.T) {
		f.Action = "payment_recorded"
		f.Entity = domain.PaymentEntity
		f.EntityID = p.Payment.ID
		f.ActorID = owner.Staff.ID
		f.RequestID = "server-audit-request"
		f.Source = "business"
		v, err := trail.List(ctx, owner.Staff.ID, f)
		if err != nil || len(v.Page.Items) != 1 {
			t.Fatal(v, err)
		}
		f.From, f.To = fixed.Add(-time.Hour), fixed
		v, err = trail.List(ctx, owner.Staff.ID, f)
		if err != nil || len(v.Page.Items) != 0 || v.Page.Items == nil {
			t.Fatal("exclusive upper date", v, err)
		}
		f = repository.AuditFilter{From: fixed, To: fixed.Add(time.Hour), Page: repository.Page{Limit: 2}}
		v, err = trail.List(ctx, owner.Staff.ID, f)
		if err != nil || len(v.Page.Items) != 2 || !v.Page.HasMore {
			t.Fatal(v, err)
		}
		ids := map[string]bool{}
		count := 0
		for {
			v, err := trail.List(ctx, owner.Staff.ID, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range v.Page.Items {
				key := e.Source + e.ID
				if ids[key] {
					t.Fatal("duplicate pagination")
				}
				ids[key] = true
				count++
			}
			if !v.Page.HasMore {
				break
			}
			f.Page.Offset += 2
		}
		if count <= 10 {
			t.Fatal("missing page rows", count)
		}
		f.Page.Offset = 10000
		v, err = trail.List(ctx, owner.Staff.ID, f)
		if err != nil || v.Page.Items == nil || len(v.Page.Items) != 0 || v.Page.HasMore {
			t.Fatal("empty page", v, err)
		}
	})
	t.Run("failed-transactions-no-success-audit", func(t *testing.T) {
		newID := create("rollback-order")
		newPayment, err := payments.Record(ctx, cashier.Staff.ID, PaymentInput{OrderID: newID, Amount: 6000, Method: domain.QRIS, IdempotencyKey: "partial"})
		if err != nil {
			t.Fatal(err)
		}
		active, err := shifts.Open(ctx, owner.Staff.ID, ShiftOpenInput{Key: "rollback-shift", Opening: 0})
		if err != nil {
			t.Fatal(err)
		}
		fail := refundFailureWork{uow, "audit"}
		cases := []struct {
			name string
			run  func() error
		}{
			{"staff", func() error {
				_, err := NewStaffManagement(fail, rt, platform.Passwords{}).Create(ctx, owner.Staff.ID, StaffInput{Username: "rollback.staff", Name: "Rollback", Role: domain.Admin, Password: password})
				return err
			}},
			{"price", func() error {
				_, err := NewCatalog(fail, rt).Configure(ctx, owner.Staff.ID, ServiceInput{ID: svc.Service.ID, ExpectedVersion: 2, Name: "Rollback", Unit: domain.PerPiece, UnitPrice: 15000, Turnaround: time.Hour, Active: true})
				return err
			}},
			{"status", func() error {
				current, err := orders.Get(ctx, owner.Staff.ID, newID)
				if err != nil {
					return err
				}
				_, err = NewOrders(fail, rt).ChangeStatus(ctx, owner.Staff.ID, newID, domain.Processing, current.Version, "rollback-status")
				return err
			}},
			{"payment", func() error {
				_, err := NewPayments(fail, rt).Record(ctx, cashier.Staff.ID, PaymentInput{OrderID: newID, Amount: 1000, Method: domain.QRIS, IdempotencyKey: "rollback-payment"})
				return err
			}},
			{"refund", func() error {
				_, err := NewPayments(fail, rt).Refund(ctx, owner.Staff.ID, RefundInput{OrderID: newID, PaymentID: newPayment.Payment.ID, IdempotencyKey: "rollback-refund", Reason: "Alasan"})
				return err
			}},
			{"close", func() error {
				_, err := NewShifts(fail, rt).Close(ctx, owner.Staff.ID, ShiftCloseInput{ID: active.Shift.ID, Key: "rollback-close", Physical: 0})
				return err
			}},
		}
		var before, after string
		snapshot := func() string {
			t.Helper()
			var s string
			err := db.QueryRowContext(ctx, `SELECT jsonb_build_object('staff',(SELECT jsonb_agg(s ORDER BY id) FROM staff s),'services',(SELECT jsonb_agg(s ORDER BY id) FROM services s),'orders',(SELECT jsonb_agg(s ORDER BY id) FROM orders s),'payments',(SELECT jsonb_agg(s ORDER BY id) FROM payments s),'refunds',(SELECT jsonb_agg(s ORDER BY id) FROM refunds s),'ledger',(SELECT jsonb_agg(s ORDER BY id) FROM ledger_entries s),'shifts',(SELECT jsonb_agg(s ORDER BY id) FROM cash_shifts s),'audit',(SELECT jsonb_agg(s ORDER BY id) FROM audit_events s))::text`).Scan(&s)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				before = snapshot()
				err := tc.run()
				if !errors.Is(err, failure) {
					t.Fatal("failure not propagated", err)
				}
				after = snapshot()
				if after != before {
					t.Fatal("failed transaction changed data/audit")
				}
			})
		}
	})
}
