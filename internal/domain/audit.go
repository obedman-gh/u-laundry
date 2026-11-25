package domain

import "time"

type AuditAction string
type AuditEntity string

const (
	CredentialsVerified   AuditAction = "credentials_verified"
	ShiftOpened           AuditAction = "shift_opened"
	ShiftClosed           AuditAction = "shift_closed"
	ShiftCashMoved        AuditAction = "shift_cash_moved"
	ShiftEntity           AuditEntity = "shift"
	CustomerCreated       AuditAction = "customer_created"
	ServiceConfigured     AuditAction = "service_configured"
	OrderCancelled        AuditAction = "order_cancelled"
	ReportGenerated       AuditAction = "report_generated"
	StaffCreated          AuditAction = "staff_created"
	StaffBootstrapped     AuditAction = "staff_bootstrapped"
	StaffDeactivated      AuditAction = "staff_deactivated"
	StaffActivated        AuditAction = "staff_activated"
	StaffRoleChanged      AuditAction = "staff_role_changed"
	StaffPasswordReset    AuditAction = "staff_password_reset"
	StaffLoggedIn         AuditAction = "staff_logged_in"
	ReportEntity          AuditEntity = "report"
	OrderCreated          AuditAction = "order_created"
	OrderTrackingRotated  AuditAction = "order_tracking_rotated"
	OrderStatusChanged    AuditAction = "order_status_changed"
	PaymentRecorded       AuditAction = "payment_recorded"
	PaymentRefundRecorded AuditAction = "payment_refunded"
	RefundEntity          AuditEntity = "refund"
	DiscountApproved      AuditAction = "discount_approved"
	CustomerUpdated       AuditAction = "customer_updated"
	ServiceUpdated        AuditAction = "service_updated"
	StaffUpdated          AuditAction = "staff_updated"
	OrderEntity           AuditEntity = "order"
	PaymentEntity         AuditEntity = "payment"
	CustomerEntity        AuditEntity = "customer"
	ServiceEntity         AuditEntity = "service"
	StaffEntity           AuditEntity = "staff"
)

func (a AuditAction) entity() AuditEntity {
	switch a {
	case ShiftOpened, ShiftClosed, ShiftCashMoved:
		return ShiftEntity
	case OrderCreated, OrderStatusChanged, OrderCancelled, DiscountApproved, OrderTrackingRotated:
		return OrderEntity
	case PaymentRecorded:
		return PaymentEntity
	case PaymentRefundRecorded:
		return RefundEntity
	case CustomerCreated, CustomerUpdated:
		return CustomerEntity
	case ServiceConfigured, ServiceUpdated:
		return ServiceEntity
	case StaffUpdated, StaffLoggedIn, CredentialsVerified, StaffCreated, StaffBootstrapped, StaffDeactivated, StaffActivated, StaffRoleChanged, StaffPasswordReset:
		return StaffEntity
	case ReportGenerated:
		return ReportEntity
	default:
		return ""
	}
}

// AuditEvent has only controlled metadata, never request bodies or PII snapshots.
type AuditEvent struct {
	ID, ActorID, EntityID string
	RequestID             string
	Metadata              AuditMetadata
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
	if e.RequestID != "" && !ValidAuditRequestID(e.RequestID) {
		return invalid("audit.request_id", "Identitas permintaan audit tidak valid.")
	}
	return e.Metadata.Validate()
}

func (a AuditAction) Valid() bool { return a.entity() != "" }
func (e AuditEntity) Valid() bool {
	switch e {
	case ShiftEntity, OrderEntity, PaymentEntity, RefundEntity, CustomerEntity, ServiceEntity, StaffEntity, ReportEntity:
		return true
	default:
		return false
	}
}
func ValidAuditRequestID(id string) bool { return identifier("request_id", id) == nil }

type AuditMetadata struct {
	Version   int64       `json:"version,omitempty"`
	Status    OrderStatus `json:"status,omitempty"`
	Role      Role        `json:"role,omitempty"`
	Active    *bool       `json:"active,omitempty"`
	UnitPrice *Money      `json:"unit_price,omitempty"`
}

func (m AuditMetadata) Validate() error {
	if m.Version < 0 || (m.Status != "" && !m.Status.Valid()) || (m.Role != "" && m.Role != Owner && m.Role != Admin && m.Role != Cashier) || (m.UnitPrice != nil && *m.UnitPrice < 0) {
		return invalid("audit.metadata", "Metadata audit tidak valid.")
	}
	return nil
}
