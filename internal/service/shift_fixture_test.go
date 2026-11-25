package service

import (
	"context"
	"ulaundry/internal/domain"
)

// Existing fake-UoW payment tests have a synthetic open drawer. Real shift
// absence, ownership, locking and close races are covered against PostgreSQL.
func (t *fakeTx) LockPostingShift(_ context.Context, actor string, method domain.PaymentMethod) (string, error) {
	if err := t.call("lock_posting_shift"); err != nil {
		return "", err
	}
	if method != domain.Cash {
		return "", nil
	}
	return "fixture-shift-" + actor, nil
}
func (t *fakeTx) GetShift(_ context.Context, id string) (domain.CashShift, error) {
	return domain.CashShift{ID: id, CashierID: "actor-1", OpenKey: "fixture-open", OpenedAt: fixed}, nil
}
func (t *fakeTx) ShiftTotals(_ context.Context, id string) (domain.ShiftTotals, error) {
	var v domain.ShiftTotals
	for _, e := range t.state.ledger {
		if e.Method != domain.Cash {
			continue
		}
		if e.Type == domain.Debit {
			v.CashPayments += e.Amount
		} else {
			v.CashRefunds += e.Amount
		}
	}
	return v, nil
}
