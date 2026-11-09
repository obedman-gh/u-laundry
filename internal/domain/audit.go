package domain

import "time"

type AuditAction string
type AuditEntity string

const (
	CustomerCreated      AuditAction = "customer_created"
	ServiceConfigured    AuditAction = "service_configured"
	OrderCancelled       AuditAction = "order_cancelled"
	ReportGenerated      AuditAction = "report_generated"
	StaffCreated         AuditAction = "staff_created"
	StaffBootstrapped    AuditAction = "staff_bootstrapped"
	StaffDeactivated     AuditAction = "staff_deactivated"
	StaffActivated       AuditAction = "staff_activated"
	StaffRoleChanged     AuditAction = "staff_role_changed"
	StaffPasswordReset   AuditAction = "staff_password_reset"
	StaffLoggedIn        AuditAction = "staff_logged_in"
	ReportEntity         AuditEntity = "report"
	OrderCreated         AuditAction = "order_created"
	OrderTrackingRotated AuditAction = "order_tracking_rotated"
	OrderStatusChanged   AuditAction = "order_status_changed"
	PaymentRecorded      AuditAction = "payment_recorded"
	DiscountApproved     AuditAction = "discount_approved"
	CustomerUpdated      AuditAction = "customer_updated"
	ServiceUpdated       AuditAction = "service_updated"
	StaffUpdated         AuditAction = "staff_updated"
	OrderEntity          AuditEntity = "order"
	PaymentEntity        AuditEntity = "payment"
	CustomerEntity       AuditEntity = "customer"
	ServiceEntity        AuditEntity = "service"
	StaffEntity          AuditEntity = "staff"
)

func (a AuditAction) entity() AuditEntity {
	switch a {
	case OrderCreated, OrderStatusChanged, OrderCancelled, DiscountApproved, OrderTrackingRotated:
		return OrderEntity
	case PaymentRecorded:
		return PaymentEntity
	case CustomerCreated, CustomerUpdated:
		return CustomerEntity
	case ServiceConfigured, ServiceUpdated:
		return ServiceEntity
	case StaffUpdated, StaffLoggedIn, StaffCreated, StaffBootstrapped, StaffDeactivated, StaffActivated, StaffRoleChanged, StaffPasswordReset:
		return StaffEntity
	case ReportGenerated:
		return ReportEntity
	default:
		return ""
	}
}

// AuditEvent deliberately contains only actor/entity IDs and a controlled action.
// No free-form metadata, passwords, tokens, customer names or contact snapshots.
// Append-only storage and transactional event emission are later service concerns.
type AuditEvent struct {
	ID, ActorID, EntityID string
	Action                AuditAction
	Entity                AuditEntity
	OccurredAt            time.Time
}

func (e AuditEvent) Validate() error {
	for _, item := range []struct{ field, value string }{
		{"audit.id", e.ID}, {"audit.actor_id", e.ActorID}, {"audit.entity_id", e.EntityID},
	} {
		if err := identifier(item.field, item.value); err != nil {
			return err
		}
	}
	if e.Action.entity() == "" || e.Action.entity() != e.Entity {
		return invalid("audit.action", "Aksi audit dan jenis objek tidak sesuai.")
	}
	if e.OccurredAt.IsZero() {
		return invalid("audit.occurred_at", "Waktu audit wajib diisi.")
	}
	return nil
}
