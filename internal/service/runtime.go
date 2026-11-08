// Package service owns workflows, authorization intent, and transaction scope.
// It knows repository contracts and domain rules, never SQL or HTTP.
package service

import (
	"context"
	"errors"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type Clock interface{ Now() time.Time }
type Generator interface{ New() (string, error) }
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }

type GeneratorFunc func() (string, error)

func (f GeneratorFunc) New() (string, error) { return f() }

type Runtime struct {
	Clock  Clock
	IDs    Generator
	Tokens Generator
}

func (r Runtime) now() (time.Time, error) {
	if r.Clock == nil {
		return time.Time{}, errors.New("Clock belum tersedia")
	}
	at := r.Clock.Now().UTC().Truncate(time.Microsecond)
	if at.IsZero() {
		return at, invalid("clock", "Waktu server tidak valid.")
	}
	return at, nil
}
func (r Runtime) id() (string, error) {
	if r.IDs == nil {
		return "", errors.New("Generator ID belum tersedia")
	}
	id, err := r.IDs.New()
	if err != nil {
		return "", err
	}
	if len(id) < 1 || len(id) > 100 {
		return "", errors.New("Generator menghasilkan ID tidak valid")
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return "", errors.New("Generator menghasilkan ID tidak valid")
		}
	}
	return id, nil
}
func invalid(field, message string) error {
	return &domain.Error{Kind: domain.Validation, Code: "invalid_value", Field: field, Message: message}
}
func forbidden() error {
	return &domain.Error{Kind: domain.Forbidden, Code: "forbidden", Field: "actor", Message: "Staf tidak diizinkan menjalankan tindakan ini."}
}
func conflict(code, field, message string) error {
	return &domain.Error{Kind: domain.Conflict, Code: code, Field: field, Message: message}
}

type Action string

const (
	ManageStaff         Action = "manage_staff"
	ReadAudit           Action = "read_audit"
	ManageConfiguration Action = "manage_configuration"
	ApproveDiscount     Action = "approve_discount"
	CreateCustomer      Action = "create_customer"
	ConfigureService    Action = "configure_service"
	CreateOrder         Action = "create_order"
	ChangeStatus        Action = "change_status"
	RecordPayment       Action = "record_payment"
	CancelOrder         Action = "cancel_order"
	ReadOrders          Action = "read_orders"
	GenerateReport      Action = "generate_report"
)

func Allowed(role domain.Role, action Action) bool {
	switch action {
	case ManageStaff, ManageConfiguration:
		return role == domain.Owner
	case ConfigureService, CancelOrder, GenerateReport, ReadAudit, ApproveDiscount:
		return role == domain.Owner || role == domain.Admin
	case CreateCustomer, CreateOrder, ChangeStatus, RecordPayment, ReadOrders:
		return role == domain.Owner || role == domain.Admin || role == domain.Cashier
	default:
		return false
	}
}
func authorize(ctx context.Context, r repository.UserLookup, id string, action Action) (domain.Staff, error) {
	if id == "" {
		return domain.Staff{}, &AccessDenied{Permission: action}
	}
	a, err := r.GetStaff(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.Staff{}, &AccessDenied{Permission: action}
	}
	if err != nil {
		return domain.Staff{}, err
	}
	if !a.Active || a.Validate() != nil || !Allowed(a.Role, action) {
		return domain.Staff{}, &AccessDenied{ActorID: a.ID, Permission: action}
	}
	return a, nil
}
func audit(ctx context.Context, tx repository.AuditAppender, rt Runtime, actor string, action domain.AuditAction, entity domain.AuditEntity, id string, at time.Time) error {
	eventID, err := rt.id()
	if err != nil {
		return err
	}
	return tx.AppendAudit(ctx, domain.AuditEvent{ID: eventID, ActorID: actor, Action: action, Entity: entity, EntityID: id, OccurredAt: at})
}
