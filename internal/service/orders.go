package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type Orders struct {
	uow     repository.UnitOfWork
	runtime Runtime
}

func NewOrders(uow repository.UnitOfWork, rt Runtime) *Orders { return &Orders{uow, rt} }

type OrderInput struct {
	CustomerID            string
	Items                 []domain.ItemRequest
	Discount              domain.Money
	DiscountReason, Notes string
	EstimatedReadyAt      time.Time
}
type CreatedOrder struct {
	Order         domain.Order
	TrackingToken string
}

func tokenHash(token string) ([32]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(token)), true
}
func (s *Orders) Create(ctx context.Context, actor string, in OrderInput) (CreatedOrder, error) {
	var result CreatedOrder
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, CreateOrder)
		if err != nil {
			return err
		}
		if len(in.Items) == 0 || len(in.Items) > 100 {
			return invalid("items", "Pesanan harus berisi 1–100 item.")
		}
		customer, err := tx.GetCustomer(ctx, in.CustomerID)
		if err != nil {
			return err
		}
		catalog := make([]domain.Service, 0, len(in.Items))
		seen := map[string]bool{}
		for _, item := range in.Items {
			if seen[item.ServiceID] {
				continue
			}
			seen[item.ServiceID] = true
			v, err := tx.GetService(ctx, item.ServiceID)
			if err != nil {
				return err
			}
			catalog = append(catalog, v)
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		id, err := s.runtime.id()
		if err != nil {
			return err
		}
		estimate := in.EstimatedReadyAt
		if !estimate.IsZero() {
			estimate = estimate.UTC().Truncate(time.Microsecond)
		}
		order, err := domain.NewOrder(domain.NewOrderInput{ID: id, Items: in.Items, Discount: in.Discount, DiscountReason: in.DiscountReason, Notes: in.Notes, CreatedAt: at, EstimatedReadyAt: estimate}, customer, catalog, a)
		if err != nil {
			return err
		}
		if s.runtime.Tokens == nil {
			return errors.New("Generator tracking belum tersedia")
		}
		token, err := s.runtime.Tokens.New()
		if err != nil {
			return err
		}
		hash, ok := tokenHash(token)
		if !ok {
			return errors.New("Generator menghasilkan token tracking tidak valid")
		}
		if err := tx.CreateOrder(ctx, order); err != nil {
			return err
		}
		if err := tx.AddTracking(ctx, order.ID(), hash); err != nil {
			return err
		}
		if err := audit(ctx, tx, s.runtime, a.ID, domain.OrderCreated, domain.OrderEntity, order.ID(), at); err != nil {
			return err
		}
		if in.Discount > 0 {
			if err := audit(ctx, tx, s.runtime, a.ID, domain.DiscountApproved, domain.OrderEntity, order.ID(), at); err != nil {
				return err
			}
		}
		result = CreatedOrder{Order: order, TrackingToken: token}
		return nil
	})
	if err != nil {
		return CreatedOrder{}, err
	}
	return result, nil
}
func (s *Orders) ChangeStatus(ctx context.Context, actor, id string, to domain.OrderStatus, expected int64) (repository.OrderRecord, error) {
	if to == domain.Cancelled {
		return repository.OrderRecord{}, invalid("status", "Gunakan pembatalan dengan alasan yang tercatat.")
	}
	return s.transition(ctx, actor, id, to, expected, "", ChangeStatus)
}
func (s *Orders) Cancel(ctx context.Context, actor, id string, expected int64, reason string) (repository.OrderRecord, error) {
	return s.transition(ctx, actor, id, domain.Cancelled, expected, reason, CancelOrder)
}
func (s *Orders) transition(ctx context.Context, actor, id string, to domain.OrderStatus, expected int64, reason string, action Action) (repository.OrderRecord, error) {
	var result repository.OrderRecord
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, action)
		if err != nil {
			return err
		}
		r, err := tx.LockOrder(ctx, id)
		if err != nil {
			return err
		}
		if expected < 1 || expected != r.Version {
			return conflict("stale_order", "version", "Versi pesanan telah berubah.")
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		if err := r.Order.Transition(to, at, reason); err != nil {
			return err
		}
		if err := tx.SaveOrderProgress(ctx, r.Order, r.Version); err != nil {
			return err
		}
		auditAction := domain.OrderStatusChanged
		if action == CancelOrder {
			auditAction = domain.OrderCancelled
		}
		if err := audit(ctx, tx, s.runtime, a.ID, auditAction, domain.OrderEntity, id, at); err != nil {
			return err
		}
		r.Version++
		result = r
		return nil
	})
	if err != nil {
		return repository.OrderRecord{}, err
	}
	return result, nil
}
func (s *Orders) Get(ctx context.Context, actor, id string) (repository.OrderRecord, error) {
	var result repository.OrderRecord
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadOrders); err != nil {
			return err
		}
		var err error
		result, err = r.GetOrder(ctx, id)
		return err
	})
	if err != nil {
		return repository.OrderRecord{}, err
	}
	return result, nil
}

type OrderListItem struct {
	repository.OrderSummary
	PaymentStatus domain.PaymentStatus
}

func (s *Orders) List(ctx context.Context, actor string, filter repository.OrderFilter) ([]OrderListItem, error) {
	var result []repository.OrderSummary
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadOrders); err != nil {
			return err
		}
		var err error
		result, err = r.ListOrders(ctx, filter)
		return err
	})
	if err != nil {
		return nil, err
	}
	views := make([]OrderListItem, 0, len(result))
	for _, item := range result {
		if item.Total < 0 || item.Paid < 0 || item.Paid > item.Total {
			return nil, &repository.Error{Kind: repository.InvalidData, Operation: "list orders"}
		}
		status := domain.Unpaid
		if item.Paid == item.Total {
			status = domain.Paid
		} else if item.Paid > 0 {
			status = domain.PartiallyPaid
		}
		views = append(views, OrderListItem{item, status})
	}
	return views, nil
}
