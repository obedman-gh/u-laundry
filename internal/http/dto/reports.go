package dto

import "time"

type ReportMeta struct {
	ID       string    `json:"id"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	AsOf     time.Time `json:"as_of"`
	Timezone string    `json:"timezone"`
}
type ReportOrders struct {
	Created          int64 `json:"created"`
	Received         int64 `json:"received"`
	Processing       int64 `json:"processing"`
	Ready            int64 `json:"ready"`
	Overdue          int64 `json:"overdue"`
	ReceivableOrders int64 `json:"receivable_orders"`
	Receivable       int64 `json:"receivable"`
}
type ReportMoney struct {
	Payments           int64 `json:"payments"`
	Refunds            int64 `json:"refunds"`
	Net                int64 `json:"net"`
	Expenses           int64 `json:"expenses"`
	CashIn             int64 `json:"cash_in"`
	CashOut            int64 `json:"cash_out"`
	UnassignedPayments int64 `json:"unassigned_payments"`
	UnassignedRefunds  int64 `json:"unassigned_refunds"`
}
type ReportMethod struct {
	Method   string `json:"method"`
	Payments int64  `json:"payments"`
	Refunds  int64  `json:"refunds"`
}
type ReportShifts struct {
	Opened     int64 `json:"opened"`
	Closed     int64 `json:"closed"`
	Active     int64 `json:"active"`
	System     int64 `json:"system"`
	Physical   int64 `json:"physical"`
	Difference int64 `json:"difference"`
}
type ReportPopular struct {
	ID     string `json:"service_id"`
	Name   string `json:"name"`
	Unit   string `json:"unit"`
	Orders int64  `json:"orders"`
	Pieces int64  `json:"pieces"`
	Grams  int64  `json:"grams"`
	Gross  int64  `json:"gross_before_discount"`
}
type DashboardResponse struct {
	ReportMeta
	Orders  ReportOrders    `json:"orders"`
	Money   ReportMoney     `json:"money"`
	Methods []ReportMethod  `json:"methods"`
	Shifts  ReportShifts    `json:"shifts"`
	Popular []ReportPopular `json:"popular_services"`
}
type ReportOrder struct {
	ID               string    `json:"id"`
	CustomerName     string    `json:"customer_name"`
	Status           string    `json:"status"`
	Total            int64     `json:"total"`
	Paid             int64     `json:"paid"`
	Due              int64     `json:"due"`
	CreatedAt        time.Time `json:"created_at"`
	EstimatedReadyAt time.Time `json:"estimated_ready_at"`
}
type ReportLedger struct {
	ID          string    `json:"id"`
	OrderID     string    `json:"order_id"`
	PaymentID   string    `json:"payment_id"`
	RefundID    string    `json:"refund_id"`
	ShiftID     string    `json:"shift_id"`
	Type        string    `json:"type"`
	Method      string    `json:"method"`
	Amount      int64     `json:"amount"`
	Description string    `json:"description"`
	RecordedBy  string    `json:"recorded_by"`
	OccurredAt  time.Time `json:"occurred_at"`
}
type ReportPage[T any] struct {
	ReportMeta
	Kind    string `json:"kind"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
	HasMore bool   `json:"has_more"`
	Items   []T    `json:"items"`
}
