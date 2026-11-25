package repository

import (
	"context"
	"time"
	"ulaundry/internal/domain"
)

// Reporting projections contain no contacts, credentials or idempotency keys.
type ReportWindow struct{ From, To, AsOf time.Time }
type OrderMetrics struct {
	Created, Received, Processing, Ready, Overdue, ReceivableOrders int64
	Receivable                                                      domain.Money
}
type MoneyMetrics struct {
	Payments, Refunds, Net, Expenses, CashIn, CashOut domain.Money
	UnassignedPayments, UnassignedRefunds             domain.Money
}
type MethodMetrics struct {
	Method            string
	Payments, Refunds domain.Money
}
type ShiftMetrics struct {
	Opened, Closed, Active       int64
	System, Physical, Difference domain.Money
}
type PopularService struct {
	ID, Name              string
	Unit                  domain.Unit
	Orders, Pieces, Grams int64
	Gross                 domain.Money
}
type DashboardMetrics struct {
	Orders  OrderMetrics
	Money   MoneyMetrics
	Methods []MethodMetrics
	Shifts  ShiftMetrics
	Popular []PopularService
}
type ReportOrderFilter struct {
	Window ReportWindow
	Page   Page
	Kind   string // created (period), receivable or overdue (current snapshot)
	Status domain.OrderStatus
}
type ReportOrder struct {
	ID, CustomerName            string
	Status                      domain.OrderStatus
	Total, Paid, Due            domain.Money
	CreatedAt, EstimatedReadyAt time.Time
}
type ReportPage[T any] struct {
	Items   []T
	HasMore bool
}
type DashboardQueries interface {
	DashboardMetrics(context.Context, ReportWindow) (DashboardMetrics, error)
	ReportOrders(context.Context, ReportOrderFilter) (ReportPage[ReportOrder], error)
	ReportLedger(context.Context, ReportWindow, Page) (ReportPage[domain.LedgerEntry], error)
}
