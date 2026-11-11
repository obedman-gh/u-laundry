package domain

import (
	"fmt"
	"strings"
	"time"
)

type OrderStatus string

const (
	Received   OrderStatus = "received"
	Processing OrderStatus = "processing"
	Ready      OrderStatus = "ready"
	Collected  OrderStatus = "collected"
	Cancelled  OrderStatus = "cancelled"
)

func (s OrderStatus) Valid() bool {
	return s == Received || s == Processing || s == Ready || s == Collected || s == Cancelled
}

// CheckTransition checks the operational graph only. Transition also enforces
// payment, cancellation reason, and chronology for an actual order.
func CheckTransition(from, to OrderStatus) error {
	if !from.Valid() || !to.Valid() {
		return invalid("status", "Status operasional tidak dikenal.")
	}
	allowed := (from == Received && (to == Processing || to == Cancelled)) ||
		(from == Processing && (to == Ready || to == Cancelled)) ||
		(from == Ready && to == Collected)
	if !allowed {
		return conflict("invalid_transition", "status", "Perubahan status pesanan tidak diizinkan.")
	}
	return nil
}

// Quantity uses whole pieces or integer grams, never fractional float kilograms.
// Exactly one field is positive and must match the service unit.
type Quantity struct{ Pieces, Grams int64 }

func (q Quantity) Validate(unit Unit) error {
	if (unit == PerPiece && q.Pieces > 0 && q.Grams == 0) ||
		(unit == PerKilogram && q.Grams > 0 && q.Pieces == 0) {
		return nil
	}
	return invalid("quantity", "Isi jumlah satuan atau berat gram positif sesuai layanan.")
}

type OrderItem struct {
	ServiceID, ServiceName string
	Unit                   Unit
	UnitPrice              Money
	Quantity               Quantity
	LineTotal              Money
}

// Validate is also available to future repository hydration code: snapshots
// must agree with their calculated amount, even when not constructed here.
func (item OrderItem) Validate() error {
	if err := identifier("item.service_id", item.ServiceID); err != nil {
		return err
	}
	if err := text("item.service_name", item.ServiceName, 100, true, false); err != nil {
		return err
	}
	if err := item.Quantity.Validate(item.Unit); err != nil {
		return err
	}
	var expected Money
	var err error
	if item.Unit == PerPiece {
		expected, err = multiplyMoney(item.UnitPrice, item.Quantity.Pieces)
	} else {
		expected, err = weightTotal(item.UnitPrice, item.Quantity.Grams)
	}
	if err != nil {
		return err
	}
	if item.LineTotal != expected {
		return invalid("item.line_total", "Total item tidak sesuai harga dan jumlah.")
	}
	return nil
}

// NewOrderItem snapshots a server-loaded service and calculates its line total.
func NewOrderItem(service Service, quantity Quantity) (OrderItem, error) {
	if err := service.Validate(); err != nil {
		return OrderItem{}, err
	}
	if !service.Active {
		return OrderItem{}, conflict("service_inactive", "service", "Layanan tidak aktif.")
	}
	if err := quantity.Validate(service.Unit); err != nil {
		return OrderItem{}, err
	}
	var total Money
	var err error
	if service.Unit == PerPiece {
		total, err = multiplyMoney(service.UnitPrice, quantity.Pieces)
	} else {
		total, err = weightTotal(service.UnitPrice, quantity.Grams)
	}
	if err != nil {
		return OrderItem{}, err
	}
	return OrderItem{service.ID, service.Name, service.Unit, service.UnitPrice, quantity, total}, nil
}

type ItemRequest struct {
	ServiceID string
	Quantity  Quantity
}

type Discount struct {
	Amount             Money
	Reason, ApprovedBy string
}

// NewOrderInput intentionally has no client price, line total, order total, paid
// amount or status. catalog/customer/actor must be loaded by the trusted service.
type NewOrderInput struct {
	ID                          string
	Items                       []ItemRequest
	Discount                    Money
	DiscountReason, Notes       string
	CreatedAt, EstimatedReadyAt time.Time
}

// Order owns mutable workflow/payment state; callers receive value snapshots.
// RestoreOrder validates hydration; transaction locking belongs to persistence.
type Order struct {
	id                                              string
	customer                                        Customer
	createdBy                                       string
	items                                           []OrderItem
	subtotal, total                                 Money
	discount                                        Discount
	notes                                           string
	status                                          OrderStatus
	createdAt, updatedAt, estimatedReadyAt          time.Time
	processingAt, readyAt, collectedAt, cancelledAt time.Time
	cancellationReason                              string
	payments                                        []Payment
	paid                                            Money
}

func NewOrder(in NewOrderInput, customer Customer, catalog []Service, actor Staff) (Order, error) {
	if err := identifier("order.id", in.ID); err != nil {
		return Order{}, err
	}
	if err := customer.Validate(); err != nil {
		return Order{}, err
	}
	if err := actor.requireActive(); err != nil {
		return Order{}, err
	}
	if in.CreatedAt.IsZero() {
		return Order{}, invalid("created_at", "Waktu penerimaan wajib diisi server.")
	}
	if len(in.Items) == 0 || len(in.Items) > 100 {
		return Order{}, invalid("items", "Pesanan harus berisi 1–100 item.")
	}
	notes := cleanNotes(in.Notes)
	if err := text("notes", notes, 1000, false, true); err != nil {
		return Order{}, err
	}
	services := make(map[string]Service, len(catalog))
	for _, service := range catalog {
		if err := service.Validate(); err != nil {
			return Order{}, err
		}
		if _, duplicate := services[service.ID]; duplicate {
			return Order{}, conflict("duplicate_service", "services", "Identitas layanan katalog berulang.")
		}
		services[service.ID] = service
	}
	order := Order{id: in.ID, customer: customer, createdBy: actor.ID, notes: notes, status: Received, createdAt: in.CreatedAt, updatedAt: in.CreatedAt}
	var longest time.Duration
	for i, request := range in.Items {
		if err := identifier("service_id", request.ServiceID); err != nil {
			return Order{}, err
		}
		service, ok := services[request.ServiceID]
		if !ok {
			return Order{}, &Error{Kind: NotFound, Code: "service_not_found", Field: fmt.Sprintf("items[%d].service_id", i), Message: "Layanan tidak ditemukan."}
		}
		item, err := NewOrderItem(service, request.Quantity)
		if err != nil {
			return Order{}, err
		}
		order.subtotal, err = addMoney(order.subtotal, item.LineTotal)
		if err != nil {
			return Order{}, err
		}
		order.items = append(order.items, item)
		if service.Turnaround > longest {
			longest = service.Turnaround
		}
	}
	if err := in.Discount.Validate("discount"); err != nil {
		return Order{}, err
	}
	if in.Discount > order.subtotal {
		return Order{}, invalid("discount", "Diskon tidak boleh melebihi subtotal.")
	}
	reason := strings.TrimSpace(in.DiscountReason)
	if err := text("discount.reason", reason, 250, in.Discount > 0, false); err != nil {
		return Order{}, err
	}
	if in.Discount > 0 {
		if actor.Role != Owner && actor.Role != Admin {
			return Order{}, &Error{Kind: Forbidden, Code: "discount_approval_required", Field: "discount", Message: "Diskon memerlukan persetujuan owner atau admin."}
		}
		order.discount = Discount{Amount: in.Discount, Reason: reason, ApprovedBy: actor.ID}
	} else if reason != "" {
		return Order{}, invalid("discount.reason", "Alasan diskon hanya diisi jika ada diskon.")
	}
	order.total = order.subtotal - order.discount.Amount
	order.estimatedReadyAt = in.EstimatedReadyAt
	if order.estimatedReadyAt.IsZero() {
		order.estimatedReadyAt = in.CreatedAt.Add(longest)
	}
	if !order.estimatedReadyAt.After(in.CreatedAt) {
		return Order{}, invalid("estimated_ready_at", "Estimasi selesai harus setelah penerimaan.")
	}
	return order, nil
}

func (o Order) ID() string                  { return o.id }
func (o Order) Customer() Customer          { return o.customer }
func (o Order) CreatedBy() string           { return o.createdBy }
func (o Order) Items() []OrderItem          { return append([]OrderItem(nil), o.items...) }
func (o Order) Subtotal() Money             { return o.subtotal }
func (o Order) Discount() Discount          { return o.discount }
func (o Order) Total() Money                { return o.total }
func (o Order) Notes() string               { return o.notes }
func (o Order) Status() OrderStatus         { return o.status }
func (o Order) CreatedAt() time.Time        { return o.createdAt }
func (o Order) UpdatedAt() time.Time        { return o.updatedAt }
func (o Order) EstimatedReadyAt() time.Time { return o.estimatedReadyAt }
func (o Order) ReadyAt() time.Time          { return o.readyAt }
func (o Order) ProcessingAt() time.Time     { return o.processingAt }
func (o Order) CollectedAt() time.Time      { return o.collectedAt }
func (o Order) CancelledAt() time.Time      { return o.cancelledAt }
func (o Order) CancellationReason() string  { return o.cancellationReason }
func (o Order) Payments() []Payment         { return append([]Payment(nil), o.payments...) }
func (o Order) Paid() Money                 { return o.paid }
func (o Order) Balance() Money              { return o.total - o.paid }

func (o Order) PaymentStatus() PaymentStatus {
	if o.paid == o.total {
		return Paid
	}
	if o.paid > 0 {
		return PartiallyPaid
	}
	return Unpaid
}

func (o Order) valid() error {
	if o.id == "" || !o.status.Valid() || len(o.items) == 0 {
		return invalid("order", "Pesanan harus dibuat melalui constructor domain.")
	}
	return nil
}

func (o *Order) Transition(to OrderStatus, at time.Time, reason string) error {
	if err := o.valid(); err != nil {
		return err
	}
	if err := CheckTransition(o.status, to); err != nil {
		return err
	}
	if at.IsZero() || at.Before(o.updatedAt) {
		return invalid("transition_at", "Waktu perubahan tidak boleh mendahului pembaruan terakhir.")
	}
	reason = strings.TrimSpace(reason)
	if err := text("cancellation_reason", reason, 250, to == Cancelled, false); err != nil {
		return err
	}
	if to != Cancelled && reason != "" {
		return invalid("cancellation_reason", "Alasan pembatalan hanya berlaku saat dibatalkan.")
	}
	if to == Cancelled && o.paid != 0 {
		return conflict("payment_exists", "status", "Pesanan dengan pembayaran memerlukan penyelesaian dana sebelum pembatalan.")
	}
	if to == Collected && o.PaymentStatus() != Paid {
		return conflict("payment_required", "payment", "Pesanan harus lunas sebelum diambil.")
	}
	// Mutate only after all checks; operational transitions never post money.
	o.status, o.updatedAt = to, at
	switch to {
	case Processing:
		o.processingAt = at
	case Ready:
		o.readyAt = at
	case Collected:
		o.collectedAt = at
	case Cancelled:
		o.cancelledAt, o.cancellationReason = at, reason
	}
	return nil
}

// RecordPayment protects an in-memory order only. Durable idempotency and atomic
// payment/ledger/audit posting require a repository transaction and DB keys.
func (o *Order) RecordPayment(payment Payment) error {
	if err := o.valid(); err != nil {
		return err
	}
	if err := payment.Validate(); err != nil {
		return err
	}
	if payment.OrderID != o.id {
		return invalid("payment.order_id", "Pembayaran bukan untuk pesanan ini.")
	}
	for _, existing := range o.payments {
		if existing.ID == payment.ID || existing.IdempotencyKey == payment.IdempotencyKey {
			return conflict("duplicate_payment", "payment", "Pembayaran dengan identitas atau kunci yang sama sudah dicatat.")
		}
	}
	if o.status == Cancelled || o.status == Collected {
		return conflict("order_closed", "payment", "Pesanan telah ditutup.")
	}
	if payment.ReceivedAt.Before(o.updatedAt) {
		return invalid("payment.received_at", "Waktu pembayaran tidak boleh mendahului pembaruan terakhir.")
	}
	if payment.Amount > o.Balance() {
		return conflict("overpayment", "payment.amount", "Pembayaran melebihi sisa tagihan.")
	}
	paid, err := addMoney(o.paid, payment.Amount)
	if err != nil {
		return err
	}
	// Allocate to avoid shared backing-array mutation when an Order is copied.
	payments := append([]Payment(nil), o.payments...)
	o.payments = append(payments, payment)
	o.paid, o.updatedAt = paid, payment.ReceivedAt
	return nil
}
