package dto

import (
	"time"
	"ulaundry/internal/domain"
)

type LoginResponse struct {
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
	Name      string    `json:"nama"`
	Message   string    `json:"pesan"`
}
type HealthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Database string `json:"database"`
}
type CustomerResponse struct {
	ID   string `json:"customer_id"`
	Name string `json:"name"`
}
type ServiceResponse struct {
	ID           string `json:"service_id"`
	Version      int64  `json:"version"`
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	UnitPrice    int64  `json:"unit_price"`
	Minutes      int64  `json:"turnaround_minutes"`
	Active       bool   `json:"active"`
	DisplayOrder int    `json:"display_order"`
}
type ServiceRevisionResponse struct {
	ServiceResponse
	ChangedBy string    `json:"changed_by,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
	Source    string    `json:"source"`
}
type OrderItemResponse struct {
	ServiceID string `json:"service_id"`
	Name      string `json:"service_name"`
	Unit      string `json:"unit"`
	UnitPrice int64  `json:"unit_price"`
	Pieces    int64  `json:"pieces"`
	Grams     int64  `json:"grams"`
	LineTotal int64  `json:"line_total"`
}
type OrderResponse struct {
	Paid               int64               `json:"paid"`
	Balance            int64               `json:"balance"`
	Notes              string              `json:"notes,omitempty"`
	CancellationReason string              `json:"cancellation_reason,omitempty"`
	ID                 string              `json:"id_pesanan"`
	Customer           string              `json:"nama_customer"`
	Services           string              `json:"layanan"`
	Total              int64               `json:"total_biaya"`
	Status             string              `json:"status"`
	PaymentStatus      string              `json:"payment_status"`
	Version            int64               `json:"version"`
	CreatedAt          time.Time           `json:"waktu_pesan"`
	EstimatedReadyAt   time.Time           `json:"waktu_perkiraan_selesai"`
	ReadyAt            *time.Time          `json:"waktu_selesai_real"`
	ProcessingAt       *time.Time          `json:"processing_at"`
	CollectedAt        *time.Time          `json:"collected_at"`
	CancelledAt        *time.Time          `json:"cancelled_at"`
	Replayed           bool                `json:"replayed,omitempty"`
	AppliedVersion     int64               `json:"applied_version,omitempty"`
	Items              []OrderItemResponse `json:"items,omitempty"`
	Subtotal           *int64              `json:"subtotal,omitempty"`
	Discount           *int64              `json:"discount,omitempty"`
}
type CreatedOrderResponse struct {
	Order             OrderResponse `json:"order"`
	TrackingToken     string        `json:"tracking_token,omitempty"`
	TrackingExpiresAt *time.Time    `json:"tracking_expires_at,omitempty"`
	Replayed          bool          `json:"replayed"`
}
type OrderStatusEventResponse struct {
	Version    int64     `json:"version"`
	From       string    `json:"from_status,omitempty"`
	To         string    `json:"to_status"`
	ActorID    string    `json:"actor_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	Source     string    `json:"source"`
}
type PaymentResponse struct {
	ShiftID        string    `json:"shift_id,omitempty"`
	ID             string    `json:"payment_id"`
	Amount         int64     `json:"amount"`
	OrderID        string    `json:"order_id"`
	Method         string    `json:"method"`
	ReceivedAt     time.Time `json:"received_at"`
	CashierID      string    `json:"cashier_id"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	Replayed       bool      `json:"replayed"`
}
type RefundResponse struct {
	ShiftID        string    `json:"shift_id,omitempty"`
	ID             string    `json:"refund_id"`
	OrderID        string    `json:"order_id"`
	PaymentID      string    `json:"payment_id"`
	Amount         int64     `json:"amount"`
	Method         string    `json:"method"`
	RefundedAt     time.Time `json:"refunded_at"`
	CashierID      string    `json:"cashier_id"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	Reason         string    `json:"reason"`
	Replayed       bool      `json:"replayed"`
}
type TrackingResponse struct {
	Code             string    `json:"tracking_code"`
	Status           string    `json:"status"`
	EstimatedReadyAt time.Time `json:"estimated_ready_at"`
	Message          string    `json:"message"`
}
type CustomerSummary struct {
	ID      string `json:"customer_id"`
	Name    string `json:"name"`
	Contact string `json:"contact,omitempty"`
}
type CustomerDetail struct {
	CustomerSummary
	Address string `json:"address"`
	Notes   string `json:"notes"`
	Version int64  `json:"version"`
}
type ReportResponse struct {
	ID      string    `json:"report_id"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Inflow  int64     `json:"total_pemasukan"`
	Outflow int64     `json:"total_pengeluaran"`
	Balance int64     `json:"saldo_akhir"`
}

type StaffResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type AuditResponse struct {
	ID         string               `json:"id"`
	ActorID    string               `json:"actor_id"`
	Action     string               `json:"action"`
	Entity     string               `json:"entity"`
	EntityID   string               `json:"entity_id"`
	OccurredAt time.Time            `json:"occurred_at"`
	RequestID  string               `json:"request_id"`
	Metadata   domain.AuditMetadata `json:"metadata"`
}

type AuditRecordResponse struct {
	AuditResponse
	Source     string `json:"source"`
	Permission string `json:"permission,omitempty"`
}
type AuditHistoryResponse struct {
	From     time.Time             `json:"from"`
	To       time.Time             `json:"to"`
	Timezone string                `json:"timezone"`
	Limit    int                   `json:"limit"`
	Offset   int                   `json:"offset"`
	HasMore  bool                  `json:"has_more"`
	Items    []AuditRecordResponse `json:"items"`
}
