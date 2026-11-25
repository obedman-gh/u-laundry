package service

import (
	"context"
	"errors"
	"strings"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

const RefundPayment Action = "refund_payment"

type RefundInput struct {
	OrderID, PaymentID, IdempotencyKey, Reason string
}

func (s *Payments) Refund(ctx context.Context, actor string, in RefundInput) (repository.RefundResult, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	var result repository.RefundResult
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, RefundPayment)
		if err != nil {
			return err
		}
		if err := domain.ValidateIdempotencyKey(in.IdempotencyKey); err != nil {
			return err
		}
		o, err := tx.LockOrder(ctx, in.OrderID)
		if err != nil {
			return err
		}
		existing, err := tx.FindRefundByKey(ctx, in.OrderID, in.IdempotencyKey)
		if err == nil {
			if existing.PaymentID != in.PaymentID || existing.RecordedBy != a.ID || existing.Reason != in.Reason {
				return conflict("idempotency_conflict", "idempotency_key", "Kunci refund telah dipakai untuk data berbeda.")
			}
			result = repository.RefundResult{Refund: existing, Replayed: true}
			return nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		var p domain.Payment
		for _, payment := range o.Order.Payments() {
			if payment.ID == in.PaymentID {
				p = payment
				break
			}
		}
		if p.ID == "" {
			return &domain.Error{Kind: domain.NotFound, Code: "payment_not_found", Field: "payment_id", Message: "Pembayaran tidak ditemukan."}
		}
		shiftID, err := postingShift(ctx, tx, a.ID, p.Method, domain.Credit, p.Amount)
		if err != nil {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		id, err := s.runtime.id()
		if err != nil {
			return err
		}
		r := domain.Refund{ShiftID: shiftID, ID: id, OrderID: in.OrderID, PaymentID: p.ID, IdempotencyKey: in.IdempotencyKey, RecordedBy: a.ID, Reason: in.Reason, Amount: p.Amount, Method: p.Method, RefundedAt: at}
		if err = o.Order.RefundPayment(r); err != nil {
			return err
		}
		ledgerID, err := s.runtime.id()
		if err != nil {
			return err
		}
		result, err = tx.RecordRefund(ctx, r, ledgerID)
		if err != nil {
			return err
		}
		if result.Replayed {
			return nil
		}
		return audit(ctx, tx, s.runtime, a.ID, domain.PaymentRefundRecorded, domain.RefundEntity, r.ID, at)
	})
	if err != nil {
		return repository.RefundResult{}, err
	}
	return result, nil
}

func (s *Payments) List(ctx context.Context, actor, id string, page repository.Page) ([]domain.Payment, error) {
	var result []domain.Payment
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadOrders); err != nil {
			return err
		}
		if _, err := r.GetOrder(ctx, id); err != nil {
			return err
		}
		var err error
		result, err = r.PaymentsForOrder(ctx, id, page)
		return err
	})
	return result, err
}

func (s *Payments) Refunds(ctx context.Context, actor, id string, page repository.Page) ([]domain.Refund, error) {
	var result []domain.Refund
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadOrders); err != nil {
			return err
		}
		if _, err := r.GetOrder(ctx, id); err != nil {
			return err
		}
		var err error
		result, err = r.RefundsForOrder(ctx, id, page)
		return err
	})
	return result, err
}
