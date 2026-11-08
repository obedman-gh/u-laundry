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
	ID      string `json:"service_id"`
	Version int64  `json:"version"`
}
type OrderResponse struct {
	ID               string     `json:"id_pesanan"`
	Customer         string     `json:"nama_customer"`
	Services         string     `json:"layanan"`
	Total            int64      `json:"total_biaya"`
	Status           string     `json:"status"`
	PaymentStatus    string     `json:"payment_status"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"waktu_pesan"`
	EstimatedReadyAt time.Time  `json:"waktu_perkiraan_selesai"`
	ReadyAt          *time.Time `json:"waktu_selesai_real"`
}
type CreatedOrderResponse struct {
	Order         OrderResponse `json:"order"`
	TrackingToken string        `json:"tracking_token"`
}
type PaymentResponse struct {
	ID       string `json:"payment_id"`
	Amount   int64  `json:"amount"`
	Replayed bool   `json:"replayed"`
}
type TrackingResponse struct {
	Status           string     `json:"status"`
	EstimatedReadyAt time.Time  `json:"estimated_ready_at"`
	ReadyAt          *time.Time `json:"ready_at"`
	CollectedAt      *time.Time `json:"collected_at"`
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
