// Package handler translates HTTP requests/results. Workflow, authorization,
// calculation, audit, and transaction ownership belong to use cases.
package handler

import (
	"context"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/security"
	"ulaundry/internal/service"
)

type CustomerUseCase interface {
	Create(context.Context, string, service.CustomerInput) (domain.Customer, error)
	Search(context.Context, string, string, repository.Page) ([]domain.Customer, error)
	Get(context.Context, string, string) (repository.CustomerRecord, error)
	Update(context.Context, string, string, service.CustomerInput, int64) (repository.CustomerRecord, error)
}
type CatalogUseCase interface {
	Configure(context.Context, string, service.ServiceInput) (repository.ServiceRecord, error)
	List(context.Context, string, repository.ServiceFilter) ([]repository.ServiceRecord, error)
	Get(context.Context, string, string) (repository.ServiceRecord, error)
	History(context.Context, string, string, repository.Page) ([]repository.ServiceRevision, error)
}
type OrderUseCase interface {
	RotateTracking(context.Context, string, string, int64) (service.RotatedTracking, error)
	Create(context.Context, string, service.OrderInput) (service.CreatedOrder, error)
	ChangeStatus(context.Context, string, string, domain.OrderStatus, int64) (repository.OrderRecord, error)
	Cancel(context.Context, string, string, int64, string) (repository.OrderRecord, error)
	Get(context.Context, string, string) (repository.OrderRecord, error)
	List(context.Context, string, repository.OrderFilter) ([]service.OrderListItem, error)
}
type PaymentUseCase interface {
	Record(context.Context, string, service.PaymentInput) (repository.PaymentResult, error)
}
type TrackingUseCase interface {
	Get(context.Context, string) (service.TrackingView, error)
}
type ReportUseCase interface {
	Generate(context.Context, string, service.ReportPeriod) (service.ReportData, error)
}
type AuthUseCase interface {
	Login(context.Context, string, string) (service.LoginResult, error)
}
type HealthUseCase interface{ Check(context.Context) error }
type Dependencies struct {
	Staff     StaffUseCase
	Sessions  *security.Manager
	Customers CustomerUseCase
	Catalog   CatalogUseCase
	Orders    OrderUseCase
	Payments  PaymentUseCase
	Tracking  TrackingUseCase
	Reports   ReportUseCase
	Auth      AuthUseCase
	Health    HealthUseCase
}
type Handler struct{ use Dependencies }

func New(use Dependencies) *Handler { return &Handler{use: use} }
