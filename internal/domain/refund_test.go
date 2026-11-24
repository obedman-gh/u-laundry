package domain_test

import (
	"testing"
	"time"
	"ulaundry/internal/domain"
)

func TestRefundFullPaymentAndNetBalance(t *testing.T) {
	o := order(t)
	p := payment("p1", 5000, now.Add(time.Minute))
	if err := o.RecordPayment(p); err != nil {
		t.Fatal(err)
	}
	r := domain.Refund{ID: "r1", OrderID: p.OrderID, PaymentID: p.ID, IdempotencyKey: "r-key", RecordedBy: p.RecordedBy, Amount: p.Amount, Method: p.Method, RefundedAt: now.Add(2 * time.Minute), Reason: "Koreksi pembayaran"}
	for _, mutate := range []func(*domain.Refund){func(r *domain.Refund) { r.Amount-- }, func(r *domain.Refund) { r.Method = domain.QRIS }, func(r *domain.Refund) { r.Reason = "" }, func(r *domain.Refund) { r.RefundedAt = now }} {
		bad := r
		mutate(&bad)
		if err := o.RefundPayment(bad); err == nil {
			t.Fatal("invalid refund accepted")
		}
		if o.Paid() != 5000 {
			t.Fatal("failed refund changed balance")
		}
	}
	if err := o.RefundPayment(r); err != nil {
		t.Fatal(err)
	}
	if o.Paid() != 0 || o.PaymentStatus() != domain.Unpaid || o.Payments()[0].Status != domain.PaymentRefunded {
		t.Fatal("net balance incorrect")
	}
	if err := o.RefundPayment(r); err == nil {
		t.Fatal("duplicate refund")
	}
	if _, err := domain.RestoreOrder(o.Snapshot()); err != nil {
		t.Fatal("refund hydration", err)
	}
	l, err := domain.NewRefundLedger("r-ledger", r)
	if err != nil || l.Type != domain.Credit || l.Amount != 5000 || l.RefundID != r.ID || l.Method != p.Method {
		t.Fatal(l, err)
	}
	if err := o.Transition(domain.Cancelled, r.RefundedAt, "Permintaan pelanggan"); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.RestoreOrder(o.Snapshot()); err != nil {
		t.Fatal("cancelled refund hydration", err)
	}
}

func TestRefundCollectedOrderIsRejected(t *testing.T) {
	o := order(t)
	p := payment("p1", o.Total(), now)
	if err := o.RecordPayment(p); err != nil {
		t.Fatal(err)
	}
	for _, s := range []domain.OrderStatus{domain.Processing, domain.Ready, domain.Collected} {
		if err := o.Transition(s, now, ""); err != nil {
			t.Fatal(err)
		}
	}
	r := domain.Refund{ID: "r1", OrderID: p.OrderID, PaymentID: p.ID, IdempotencyKey: "r1", RecordedBy: p.RecordedBy, Amount: p.Amount, Method: p.Method, RefundedAt: now, Reason: "Koreksi pembayaran"}
	if err := o.RefundPayment(r); err == nil {
		t.Fatal("collected refund accepted")
	}
}
