package service

import (
	"context"
	"testing"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (t *fakeTx) FindRefundByKey(_ context.Context, id, key string) (domain.Refund, error) {
	if err := t.call("find_refund"); err != nil {
		return domain.Refund{}, err
	}
	r, ok := t.state.refunds[id+"|"+key]
	if !ok {
		return r, missing()
	}
	return r, nil
}
func (t *fakeTx) RecordRefund(_ context.Context, r domain.Refund, ledgerID string) (repository.RefundResult, error) {
	if err := t.call("record_refund"); err != nil {
		return repository.RefundResult{}, err
	}
	o := t.state.orders[r.OrderID]
	if err := o.Order.RefundPayment(r); err != nil {
		return repository.RefundResult{}, err
	}
	l, err := domain.NewRefundLedger(ledgerID, r)
	if err != nil {
		return repository.RefundResult{}, err
	}
	o.Version++
	t.state.orders[r.OrderID] = o
	if t.state.refunds == nil {
		t.state.refunds = map[string]domain.Refund{}
	}
	t.state.refunds[r.OrderID+"|"+r.IdempotencyKey] = r
	for key, p := range t.state.payments {
		if p.ID == r.PaymentID {
			p.Status = domain.PaymentRefunded
			t.state.payments[key] = p
		}
	}
	t.state.ledger[ledgerID] = l
	return repository.RefundResult{Refund: r}, nil
}

func TestRefundUseCaseReplayAuthorizationAndRollback(t *testing.T) {
	ctx := context.Background()
	setup := func() (*fakeUow, Runtime, RefundInput) {
		u, rt := fixture(t, domain.Owner)
		p, err := NewPayments(u, rt).Record(ctx, "actor-1", PaymentInput{OrderID: "order-1", IdempotencyKey: "deposit", Amount: 5000, Method: domain.QRIS})
		if err != nil {
			t.Fatal(err)
		}
		return u, rt, RefundInput{OrderID: "order-1", PaymentID: p.Payment.ID, IdempotencyKey: "refund", Reason: "Koreksi pembayaran"}
	}
	t.Run("replay_payload_conflict_and_cancel", func(t *testing.T) {
		u, rt, in := setup()
		s := NewPayments(u, rt)
		first, err := s.Refund(ctx, "actor-1", in)
		if err != nil || first.Replayed || first.Refund.Amount != 5000 || first.Refund.Method != domain.QRIS {
			t.Fatal(first, err)
		}
		before := len(u.state.audits)
		replay, err := s.Refund(ctx, "actor-1", in)
		if err != nil || !replay.Replayed || replay.Refund.ID != first.Refund.ID || len(u.state.audits) != before {
			t.Fatal(replay, err)
		}
		in.Reason = "Alasan berbeda"
		_, err = s.Refund(ctx, "actor-1", in)
		requireDomain(t, err, domain.Conflict, "idempotency_conflict")
		in.Reason = "Koreksi pembayaran"
		in.IdempotencyKey = "second-refund"
		_, err = s.Refund(ctx, "actor-1", in)
		requireDomain(t, err, domain.Conflict, "already_refunded")
		o := u.state.orders["order-1"]
		if o.Order.Paid() != 0 || o.Order.Payments()[0].Status != domain.PaymentRefunded {
			t.Fatal("refund not reflected")
		}
		if _, err := NewOrders(u, rt).Cancel(ctx, "actor-1", "order-1", o.Version, "Permintaan pelanggan", "cancel-refunded"); err != nil {
			t.Fatal(err)
		}
	})
	for _, stage := range []string{"find_refund", "record_refund", "audit", "commit"} {
		t.Run("rollback_"+stage, func(t *testing.T) {
			u, rt, in := setup()
			version := u.state.orders["order-1"].Version
			if stage == "commit" {
				u.commitErr = failure
			} else {
				u.failAt = stage
			}
			result, err := NewPayments(u, rt).Refund(ctx, "actor-1", in)
			if err == nil || result.Refund.ID != "" || len(u.state.refunds) != 0 || u.state.orders["order-1"].Version != version || u.state.orders["order-1"].Order.Paid() != 5000 || len(u.state.ledger) != 1 {
				t.Fatal("partial refund survived", err)
			}
		})
	}
	t.Run("cashier_denied", func(t *testing.T) {
		u, rt, in := setup()
		actor := u.state.staff["actor-1"]
		actor.Role = domain.Cashier
		u.state.staff[actor.ID] = actor
		_, err := NewPayments(u, rt).Refund(ctx, actor.ID, in)
		requireDomain(t, err, domain.Forbidden, "")
	})
	t.Run("replay_requires_current_refund_permission", func(t *testing.T) {
		u, rt, in := setup()
		s := NewPayments(u, rt)
		actor := u.state.staff["actor-1"]
		actor.Role = domain.Admin
		u.state.staff[actor.ID] = actor
		if _, err := s.Refund(ctx, actor.ID, in); err != nil {
			t.Fatal("admin refund denied", err)
		}
		before := len(u.state.audits)
		for _, active := range []bool{true, false} {
			actor.Role, actor.Active = domain.Cashier, active
			u.state.staff[actor.ID] = actor
			result, err := s.Refund(ctx, actor.ID, in)
			requireDomain(t, err, domain.Forbidden, "")
			if result.Refund.ID != "" || len(u.state.audits) != before || len(u.state.refunds) != 1 {
				t.Fatal("denied replay exposed or changed refund")
			}
		}
	})
}
