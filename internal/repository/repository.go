// Package repository defines use-case contracts. It depends on domain, never
// SQL, drivers, HTTP, environment settings or application-global connections.
package repository

import (
	"context"
	"time"

	"ulaundry/internal/domain"
)

// Page uses stable ordering per query. Offset is zero-based; Limit=0 means 20.
type Page struct{ Limit, Offset int }

func (p Page) Normalize() (Page, error) {
	if p.Limit == 0 {
		p.Limit = 20
	}
	if p.Limit < 1 || p.Limit > 100 || p.Offset < 0 {
		return Page{}, &domain.Error{Kind: domain.Validation, Code: "invalid_pagination", Field: "page", Message: "Limit harus 1–100 dan offset tidak boleh negatif."}
	}
	return p, nil
}

type OrderFilter struct {
	Page       Page
	Status     domain.OrderStatus // Empty means all statuses; otherwise strict enum.
	CustomerID string
}
type OrderRecord struct {
	Order          domain.Order
	Version        int64
	Replayed       bool
	AppliedVersion int64
}
type OrderSummary struct {
	ID, CustomerID, CustomerName string
	ServiceNames                 []string
	Version                      int64
	Status                       domain.OrderStatus
	Total, Paid                  domain.Money
	CreatedAt, EstimatedReadyAt  time.Time
}
type Credentials struct {
	Staff        domain.Staff
	PasswordHash string
}
type PaymentResult struct {
	Payment  domain.Payment
	Replayed bool
}

type CustomerLookup interface {
	GetCustomer(context.Context, string) (domain.Customer, error)
	GetCustomerRecord(context.Context, string) (CustomerRecord, error)
	SearchCustomers(context.Context, string, Page) ([]domain.Customer, error)
}
type CustomerRegistration interface {
	AddCustomer(context.Context, domain.Customer) error
	LockCustomer(context.Context, string) (CustomerRecord, error)
	UpdateCustomer(context.Context, domain.Customer, int64) error
}
type CustomerRecord struct {
	Customer domain.Customer
	Version  int64
}
type ServiceCatalog interface {
	GetService(context.Context, string) (domain.Service, error)
	GetServiceRecord(context.Context, string) (ServiceRecord, error)
	ListServices(context.Context, ServiceFilter) ([]ServiceRecord, error)
	ServiceHistory(context.Context, string, Page) ([]ServiceRevision, error)
	ListActiveServices(context.Context, Page) ([]domain.Service, error)
}
type ServiceRegistration interface {
	AddService(context.Context, domain.Service) error
	LockService(context.Context, string) (ServiceRecord, error)
	UpdateService(context.Context, domain.Service, int64) error
	AppendServiceRevision(context.Context, ServiceRevision) error
}
type ServiceFilter struct {
	Page            Page
	IncludeInactive bool
}
type ServiceRevision struct {
	ServiceRecord
	ActorID, Source string
}
type ServiceRecord struct {
	Service domain.Service
	Version int64
}
type TrackingRecord struct {
	Status           domain.OrderStatus
	EstimatedReadyAt time.Time
}
type TrackingLookup interface {
	GetTracking(context.Context, [32]byte, time.Time) (TrackingRecord, error)
}
type TrackingRegistration interface {
	AddTracking(context.Context, string, [32]byte, time.Time) error
	ReplaceTracking(context.Context, string, [32]byte, time.Time, int64) error
}
type LedgerTotals struct{ Inflow, Outflow domain.Money }
type ReportQueries interface {
	LedgerTotals(context.Context, time.Time, time.Time) (LedgerTotals, error)
}
type OrderQueries interface {
	GetOrder(context.Context, string) (OrderRecord, error)
	ListOrders(context.Context, OrderFilter) ([]OrderSummary, error)
	OrderHistory(context.Context, string, Page) ([]domain.OrderStatusEvent, error)
}
type OrderCommands interface {
	CreateOrder(context.Context, domain.Order) error
	LockOrder(context.Context, string) (OrderRecord, error)
	SaveOrderProgress(context.Context, domain.Order, int64) error
	AppendOrderStatusEvent(context.Context, domain.OrderStatusEvent) error
}

// Receipt and the corresponding order/audit mutations commit together. Keys are
// scoped to the actor across create/status/cancel, not to a network connection.
type OrderCommandReceipt struct {
	ActorID, Key, Operation, OrderID string
	PayloadHash                      [32]byte
	Version                          int64
}
type OrderCommandDeduplication interface {
	LockOrderCommandKey(context.Context, string, string) error
	FindOrderCommand(context.Context, string, string) (OrderCommandReceipt, error)
	SaveOrderCommand(context.Context, OrderCommandReceipt) error
}
type PaymentQueries interface {
	FindPaymentByKey(context.Context, string, string) (domain.Payment, error)
	LedgerForOrder(context.Context, string, Page) ([]domain.LedgerEntry, error)
}
type PaymentPosting interface {
	// RecordPayment locks the order and posts payment+derived ledger together.
	// Same key+business payload replays; a changed payload is a conflict.
	RecordPayment(context.Context, domain.Payment, string) (PaymentResult, error)
}
type ExpensePosting interface {
	RecordExpense(context.Context, domain.LedgerEntry) error
}
type StaffRecord struct {
	Staff   domain.Staff
	Version int64
}
type UserLookup interface {
	ListStaff(context.Context, Page) ([]StaffRecord, error)
	GetStaff(context.Context, string) (domain.Staff, error)
	FindCredentials(context.Context, string) (Credentials, error)
}
type UserRegistration interface {
	LockStaffManagement(context.Context) error
	CountStaff(context.Context, bool) (int, error)
	LockStaff(context.Context, string) (StaffRecord, error)
	UpdateStaff(context.Context, domain.Staff, int64, string) error
	AddStaff(context.Context, domain.Staff, string) error
}
type AuditReader interface {
	AuditForEntity(context.Context, domain.AuditEntity, string, Page) ([]domain.AuditEvent, error)
}
type AuditAppender interface {
	AppendAudit(context.Context, domain.AuditEvent) error
}

// Narrow interfaces above can be injected per use case. These compositions
// only scope repositories to one consistent read snapshot or one atomic write.
type Readers interface {
	CustomerLookup
	ServiceCatalog
	OrderQueries
	PaymentQueries
	UserLookup
	AuditReader
	TrackingLookup
	ReportQueries
}
type Tx interface {
	Readers
	CustomerRegistration
	ServiceRegistration
	OrderCommands
	OrderCommandDeduplication
	PaymentPosting
	ExpensePosting
	UserRegistration
	AuditAppender
	TrackingRegistration
}
type UnitOfWork interface {
	View(context.Context, func(Readers) error) error
	WithinTransaction(context.Context, func(Tx) error) error
}
