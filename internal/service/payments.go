package service

import (
	"context"
	"errors"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type Payments struct {
	uow     repository.UnitOfWork
	runtime Runtime
}

func NewPayments(uow repository.UnitOfWork, rt Runtime) *Payments { return &Payments{uow, rt} }

type PaymentInput struct {
	OrderID, IdempotencyKey string
	Amount                  domain.Money
	Method                  domain.PaymentMethod
}

func (s *Payments) Record(ctx context.Context, actor string, in PaymentInput) (repository.PaymentResult, error) {
	var result repository.PaymentResult
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, RecordPayment)
		if err != nil {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		p := domain.Payment{ID: "pending", OrderID: in.OrderID, IdempotencyKey: in.IdempotencyKey, RecordedBy: a.ID, Amount: in.Amount, Method: in.Method, ReceivedAt: at}
		if err := p.Validate(); err != nil {
			return err
		}
		// Own the business decision while holding the same order lock used by
		// posting. Check replay after locking so concurrent identical keys return
		// the original result instead of failing a stale duplicate/balance check.
		order, err := tx.LockOrder(ctx, p.OrderID)
		if err != nil {
			return err
		}
		existing, err := tx.FindPaymentByKey(ctx, p.OrderID, p.IdempotencyKey)
		if err == nil {
			if existing.Amount != p.Amount || existing.Method != p.Method || existing.RecordedBy != p.RecordedBy {
				return conflict("idempotency_conflict", "idempotency_key", "Kunci pembayaran telah dipakai untuk data berbeda.")
			}
			result = repository.PaymentResult{Payment: existing, Replayed: true}
			return nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		// Acceptance time is sampled after waiting for the order lock. A request
		// queued behind another payment must not persist an earlier timestamp.
		p.ReceivedAt,err=s.runtime.now()
		if err!=nil{return err}
		p.ID, err = s.runtime.id()
		if err != nil {
			return err
		}
		if err := order.Order.RecordPayment(p); err != nil {
			return err
		}
		ledgerID, err := s.runtime.id()
		if err != nil {
			return err
		}
		result, err = tx.RecordPayment(ctx, p, ledgerID)
		if err != nil {
			return err
		}
		if result.Replayed {
			return nil
		}
		return audit(ctx, tx, s.runtime, a.ID, domain.PaymentRecorded, domain.PaymentEntity, result.Payment.ID, result.Payment.ReceivedAt)
	})
	if err != nil {
		return repository.PaymentResult{}, err
	}
	return result, nil
}
