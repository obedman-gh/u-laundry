package domain_test

import (
	"reflect"
	"testing"
	"time"

	"ulaundry/internal/domain"
)

func TestRestoreOrderRoundTripAndValidation(t *testing.T) {
	o := order(t)
	if err := o.RecordPayment(payment("p1", 17500, now)); err != nil {
		t.Fatal(err)
	}
	for i, status := range []domain.OrderStatus{domain.Processing, domain.Ready, domain.Collected} {
		if err := o.Transition(status, now.Add(time.Duration(i+1)*time.Minute), ""); err != nil {
			t.Fatal(err)
		}
	}
	state := o.Snapshot()
	restored, err := domain.RestoreOrder(state)
	if err != nil || !reflect.DeepEqual(restored.Snapshot(), state) {
		t.Fatal("restore failed", err)
	}
	state.Items[0].UnitPrice = 1
	state.Payments[0].Amount = 1
	if restored.Items()[0].UnitPrice != 7000 || restored.Payments()[0].Amount != 17500 {
		t.Fatal("restore aliases caller state")
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.OrderState)
	}{
		{"forged_total", func(s *domain.OrderState) { s.Total++ }},
		{"forged_line", func(s *domain.OrderState) { s.Items[0].LineTotal++ }},
		{"missing_approver", func(s *domain.OrderState) { s.Discount.Amount = 1; s.Discount.Reason = "Kompensasi"; s.Total-- }},
		{"missing_milestone", func(s *domain.OrderState) { s.ReadyAt = time.Time{} }},
		{"backdated_milestone", func(s *domain.OrderState) { s.ReadyAt = now.Add(-time.Minute) }},
		{"unknown_status", func(s *domain.OrderState) { s.Status = "Sudah Lunas" }},
		{"payment_wrong_order", func(s *domain.OrderState) { s.Payments[0].OrderID = "other" }},
		{"duplicate_payment", func(s *domain.OrderState) { s.Payments = append(s.Payments, s.Payments[0]) }},
		{"overpayment", func(s *domain.OrderState) { s.Payments[0].Amount++ }},
		{"unpaid_collected", func(s *domain.OrderState) { s.Payments = nil }},
		{"payment_after_pickup", func(s *domain.OrderState) {
			s.Payments[0].ReceivedAt = s.CollectedAt.Add(time.Minute)
			s.UpdatedAt = s.Payments[0].ReceivedAt
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := o.Snapshot()
			tc.change(&s)
			_, err := domain.RestoreOrder(s)
			requireError(t, err, domain.Validation, "")
		})
	}
}
