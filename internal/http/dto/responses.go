package dto

import "time"

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
	ID               string              `json:"id_pesanan"`
	Customer         string              `json:"nama_customer"`
	Services         string              `json:"layanan"`
	Total            int64               `json:"total_biaya"`
	Status           string              `json:"status"`
	PaymentStatus    string              `json:"payment_status"`
	Version          int64               `json:"version"`
	CreatedAt        time.Time           `json:"waktu_pesan"`
	EstimatedReadyAt time.Time           `json:"waktu_perkiraan_selesai"`
	ReadyAt          *time.Time          `json:"waktu_selesai_real"`
	Items            []OrderItemResponse `json:"items,omitempty"`
	Subtotal         *int64              `json:"subtotal,omitempty"`
	Discount         *int64              `json:"discount,omitempty"`
}
type CreatedOrderResponse struct {
	Order             OrderResponse `json:"order"`
	TrackingToken     string        `json:"tracking_token"`
	TrackingExpiresAt time.Time     `json:"tracking_expires_at"`
}
type PaymentResponse struct {
	ID       string `json:"payment_id"`
	Amount   int64  `json:"amount"`
	Replayed bool   `json:"replayed"`
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
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_id"`
	Action     string    `json:"action"`
	Entity     string    `json:"entity"`
	EntityID   string    `json:"entity_id"`
	OccurredAt time.Time `json:"occurred_at"`
}
