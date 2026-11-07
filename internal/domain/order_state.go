package domain

import "time"

// OrderState is a persistence snapshot, not a client command or response DTO.
// RestoreOrder validates history without using today's mutable service prices,
// customer details, or staff permissions for an already accepted order.
type OrderState struct {
	ID, CreatedBy                          string
	Customer                               Customer
	Items                                  []OrderItem
	Subtotal, Total                        Money
	Discount                               Discount
	Notes                                  string
	Status                                 OrderStatus
	CreatedAt, UpdatedAt, EstimatedReadyAt time.Time
	ReadyAt, CollectedAt, CancelledAt      time.Time
	CancellationReason                     string
	Payments                               []Payment
}

func (o Order) Snapshot() OrderState {
	return OrderState{ID: o.id, CreatedBy: o.createdBy, Customer: o.customer, Items: o.Items(), Subtotal: o.subtotal, Total: o.total, Discount: o.discount, Notes: o.notes, Status: o.status,
		CreatedAt: o.createdAt, UpdatedAt: o.updatedAt, EstimatedReadyAt: o.estimatedReadyAt, ReadyAt: o.readyAt, CollectedAt: o.collectedAt, CancelledAt: o.cancelledAt, CancellationReason: o.cancellationReason, Payments: o.Payments()}
}

func RestoreOrder(s OrderState) (Order, error) {
	if err := identifier("order.id", s.ID); err != nil {
		return Order{}, err
	}
	if err := identifier("order.created_by", s.CreatedBy); err != nil {
		return Order{}, err
	}
	if err := s.Customer.Validate(); err != nil {
		return Order{}, err
	}
	if err := timestamps(s.CreatedAt, s.UpdatedAt); err != nil {
		return Order{}, err
	}
	if !s.EstimatedReadyAt.After(s.CreatedAt) {
		return Order{}, invalid("estimated_ready_at", "Estimasi selesai harus setelah penerimaan.")
	}
	if err := text("notes", s.Notes, 1000, false, true); err != nil {
		return Order{}, err
	}
	if len(s.Items) == 0 || len(s.Items) > 100 {
		return Order{}, invalid("items", "Pesanan harus berisi 1–100 item.")
	}
	var subtotal Money
	for _, item := range s.Items {
		if err := item.Validate(); err != nil {
			return Order{}, err
		}
		var err error
		subtotal, err = addMoney(subtotal, item.LineTotal)
		if err != nil {
			return Order{}, err
		}
	}
	if err := s.Discount.Amount.Validate("discount"); err != nil {
		return Order{}, err
	}
	if err := text("discount.reason", s.Discount.Reason, 250, s.Discount.Amount > 0, false); err != nil {
		return Order{}, err
	}
	if s.Discount.Amount > 0 {
		if err := identifier("discount.approved_by", s.Discount.ApprovedBy); err != nil {
			return Order{}, err
		}
	} else if s.Discount.Reason != "" || s.Discount.ApprovedBy != "" {
		return Order{}, invalid("discount", "Metadata diskon tidak sesuai nominal.")
	}
	if s.Subtotal != subtotal || s.Discount.Amount > subtotal || s.Total != subtotal-s.Discount.Amount {
		return Order{}, invalid("total", "Total tersimpan tidak sesuai item dan diskon.")
	}
	if !s.Status.Valid() {
		return Order{}, invalid("status", "Status operasional tidak dikenal.")
	}
	for _, at := range []time.Time{s.ReadyAt, s.CollectedAt, s.CancelledAt} {
		if !at.IsZero() && (at.Before(s.CreatedAt) || at.After(s.UpdatedAt)) {
			return Order{}, invalid("milestone", "Waktu milestone tidak sesuai riwayat pesanan.")
		}
	}
	switch s.Status {
	case Received, Processing:
		if !s.ReadyAt.IsZero() || !s.CollectedAt.IsZero() || !s.CancelledAt.IsZero() {
			return Order{}, invalid("milestone", "Milestone tidak sesuai status pesanan.")
		}
	case Ready:
		if s.ReadyAt.IsZero() || !s.CollectedAt.IsZero() || !s.CancelledAt.IsZero() {
			return Order{}, invalid("milestone", "Milestone siap tidak valid.")
		}
	case Collected:
		if s.ReadyAt.IsZero() || s.CollectedAt.IsZero() || s.CollectedAt.Before(s.ReadyAt) || !s.CancelledAt.IsZero() {
			return Order{}, invalid("milestone", "Milestone pengambilan tidak valid.")
		}
	case Cancelled:
		if s.CancelledAt.IsZero() || !s.ReadyAt.IsZero() || !s.CollectedAt.IsZero() {
			return Order{}, invalid("milestone", "Milestone pembatalan tidak valid.")
		}
	}
	if err := text("cancellation_reason", s.CancellationReason, 250, s.Status == Cancelled, false); err != nil {
		return Order{}, err
	}
	if s.Status != Cancelled && s.CancellationReason != "" {
		return Order{}, invalid("cancellation_reason", "Alasan pembatalan tidak sesuai status.")
	}
	var paid Money
	ids, keys := map[string]bool{}, map[string]bool{}
	for _, p := range s.Payments {
		if err := p.Validate(); err != nil {
			return Order{}, err
		}
		if p.OrderID != s.ID || ids[p.ID] || keys[p.IdempotencyKey] || p.ReceivedAt.Before(s.CreatedAt) || p.ReceivedAt.After(s.UpdatedAt) || (!s.CollectedAt.IsZero() && p.ReceivedAt.After(s.CollectedAt)) {
			return Order{}, invalid("payments", "Riwayat pembayaran tidak sesuai pesanan.")
		}
		ids[p.ID], keys[p.IdempotencyKey] = true, true
		var err error
		paid, err = addMoney(paid, p.Amount)
		if err != nil {
			return Order{}, err
		}
	}
	if paid > s.Total || (s.Status == Cancelled && paid > 0) || (s.Status == Collected && paid != s.Total) {
		return Order{}, invalid("payments", "Saldo pembayaran tidak sesuai status dan total.")
	}
	return Order{id: s.ID, createdBy: s.CreatedBy, customer: s.Customer, items: append([]OrderItem(nil), s.Items...), subtotal: s.Subtotal, total: s.Total, discount: s.Discount, notes: s.Notes, status: s.Status,
		createdAt: s.CreatedAt, updatedAt: s.UpdatedAt, estimatedReadyAt: s.EstimatedReadyAt, readyAt: s.ReadyAt, collectedAt: s.CollectedAt, cancelledAt: s.CancelledAt, cancellationReason: s.CancellationReason, payments: append([]Payment(nil), s.Payments...), paid: paid}, nil
}
