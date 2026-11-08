package handler

import (
	"context"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type fakeStaff struct {
	StaffUseCase
	create func(context.Context, string, service.StaffInput) (repository.StaffRecord, error)
	change func(context.Context, string, string, string, service.StaffChange) (repository.StaffRecord, error)
}

func (f fakeStaff) Create(c context.Context, a string, in service.StaffInput) (repository.StaffRecord, error) {
	return f.create(c, a, in)
}
func (f fakeStaff) Change(c context.Context, a, id, op string, in service.StaffChange) (repository.StaffRecord, error) {
	return f.change(c, a, id, op, in)
}
func TestStaffHTTPRejectsEscalationAndRedactsPasswords(t *testing.T) {
	calls := 0
	secret := "Frasa sandi sintetis"
	h := New(Dependencies{Staff: fakeStaff{create: func(_ context.Context, actor string, in service.StaffInput) (repository.StaffRecord, error) {
		calls++
		if actor != "actor-1" || in.Username != "new.test" || in.Password != secret {
			t.Fatal("wrong actor/input")
		}
		return repository.StaffRecord{Staff: domain.Staff{ID: "new-1", Username: in.Username, Name: in.Name, Role: in.Role, Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, Version: 1}, nil
	}}})
	body := `{"username":"new.test","name":"Staff","role":"cashier","password":"Frasa sandi sintetis"}`
	w := protected(t, "POST", "/api/staff", body, h.CreateStaff)
	if w.Code != 201 || calls != 1 || strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), "password") {
		t.Fatal("create response leaked credentials", w.Code)
	}
	for _, extra := range []string{`,"actor_id":"owner"`, `,"password_hash":"fake"`, `,"active":false`} {
		w = protected(t, "POST", "/api/staff", strings.TrimSuffix(body, "}")+extra+"}", h.CreateStaff)
		checkError(t, w, 400, "unknown_field")
		if calls != 1 {
			t.Fatal("untrusted field reached service")
		}
	}
	h = New(Dependencies{Staff: fakeStaff{change: func(context.Context, string, string, string, service.StaffChange) (repository.StaffRecord, error) {
		return repository.StaffRecord{}, &domain.Error{Kind: domain.Forbidden, Code: "forbidden"}
	}}})
	w = protected(t, "POST", "/api/staff/order-1/reset-password", `{"password":"Frasa sandi sintetis","version":1}`, h.ResetStaffPassword)
	checkError(t, w, 403, "forbidden")
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("password reflected")
	}
	w = protected(t, "PUT", "/api/staff/order-1/active", `{"version":1}`, h.ChangeStaffActive)
	checkError(t, w, 400, "validation_failed")
	w = protected(t, "PUT", "/api/staff/order-1/role", `{"role":"superadmin","version":1}`, h.ChangeStaffRole)
	checkError(t, w, 400, "validation_failed")
}
