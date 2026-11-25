package domain

import "time"

type PaymentStatus string

const (
	Unpaid        PaymentStatus = "unpaid"
	PartiallyPaid PaymentStatus = "partially_paid"
	Paid          PaymentStatus = "paid"
)

type PaymentMethod string

type PaymentRecordStatus string

const (
	PaymentPosted   PaymentRecordStatus = "posted"
	PaymentRefunded PaymentRecordStatus = "refunded"
)

const (
	Cash         PaymentMethod = "cash"
	BankTransfer PaymentMethod = "bank_transfer"
	QRIS         PaymentMethod = "qris"
)

// Payment is a recorded positive receipt, not a pending gateway transaction.
// ReceivedAt and RecordedBy are supplied by the trusted server use case.
type Payment struct {
	ShiftID                                 string
	ID, OrderID, IdempotencyKey, RecordedBy string
	Amount                                  Money
	Method                                  PaymentMethod
	Status                                  PaymentRecordStatus
	ReceivedAt                              time.Time
}

func (p Payment) Validate() error {
	if p.ShiftID != "" {
		if err := identifier("payment.shift_id", p.ShiftID); err != nil {
			return err
		}
	}
	for _, item := range []struct{ field, value string }{
		{"payment.id", p.ID}, {"payment.order_id", p.OrderID},
		{"payment.idempotency_key", p.IdempotencyKey}, {"payment.recorded_by", p.RecordedBy},
	} {
		if err := identifier(item.field, item.value); err != nil {
			return err
		}
	}
	if p.Amount <= 0 {
		return invalid("payment.amount", "Pembayaran harus lebih dari nol rupiah.")
	}
	if p.Status != PaymentPosted && p.Status != PaymentRefunded {
		return invalid("payment.status", "Status record pembayaran tidak dikenal.")
	}
	if p.Method != Cash && p.Method != BankTransfer && p.Method != QRIS {
		return invalid("payment.method", "Metode pembayaran tidak dikenal.")
	}
	if p.ReceivedAt.IsZero() {
		return invalid("payment.received_at", "Waktu pembayaran wajib diisi.")
	}
	return nil
}

type LedgerType string

const (
	Debit  LedgerType = "DEBIT"  // Cash inflow, matching the existing cash book.
	Credit LedgerType = "KREDIT" // Cash outflow; not double-entry accounting.
)

type LedgerEntry struct {
	ShiftID                                                   string
	ID, OrderID, PaymentID, RefundID, RecordedBy, Description string
	Type                                                      LedgerType
	Method                                                    PaymentMethod
	Amount                                                    Money
	OccurredAt                                                time.Time
}

func (e LedgerEntry) Validate() error {
	if e.ShiftID != "" {
		if err := identifier("shift_id", e.ShiftID); err != nil {
			return err
		}
	}
	if err := identifier("ledger.id", e.ID); err != nil {
		return err
	}
	if err := identifier("ledger.recorded_by", e.RecordedBy); err != nil {
		return err
	}
	if err := text("ledger.description", e.Description, 250, true, false); err != nil {
		return err
	}
	if e.Type != Debit && e.Type != Credit {
		return invalid("ledger.type", "Tipe buku kas harus DEBIT atau KREDIT.")
	}
	if e.Amount <= 0 {
		return invalid("ledger.amount", "Nominal buku kas harus positif.")
	}
	if e.OccurredAt.IsZero() {
		return invalid("ledger.occurred_at", "Waktu buku kas wajib diisi.")
	}
	if e.OrderID != "" {
		if err := identifier("ledger.order_id", e.OrderID); err != nil {
			return err
		}
	}
	if e.PaymentID != "" {
		if err := identifier("ledger.payment_id", e.PaymentID); err != nil {
			return err
		}
		if e.OrderID == "" || e.Type != Debit {
			return invalid("ledger.payment", "Penerimaan pembayaran wajib terkait pesanan dan bertipe DEBIT.")
		}
	}
	if e.RefundID != "" {
		if err := identifier("ledger.refund_id", e.RefundID); err != nil {
			return err
		}
		if e.OrderID == "" || e.Type != Credit || e.PaymentID != "" {
			return invalid("ledger.refund", "Refund wajib terkait pesanan dan bertipe KREDIT.")
		}
	}
	if e.Method != "" && e.Method != Cash && e.Method != BankTransfer && e.Method != QRIS || (e.PaymentID != "" || e.RefundID != "") && e.Method == "" {
		return invalid("ledger.method", "Metode buku kas tidak valid.")
	}
	return nil
}

// NewPaymentLedger copies the payment's authoritative amount and references.
// It does not persist anything or itself guarantee one posting per payment.
func NewPaymentLedger(id string, payment Payment) (LedgerEntry, error) {
	if err := payment.Validate(); err != nil {
		return LedgerEntry{}, err
	}
	if payment.Status != PaymentPosted {
		return LedgerEntry{}, invalid("payment.status", "Posting hanya untuk pembayaran baru.")
	}
	entry := LedgerEntry{ShiftID: payment.ShiftID, ID: id, OrderID: payment.OrderID, PaymentID: payment.ID, RecordedBy: payment.RecordedBy, Description: "Penerimaan pembayaran pesanan", Type: Debit, Method: payment.Method, Amount: payment.Amount, OccurredAt: payment.ReceivedAt}
	if err := entry.Validate(); err != nil {
		return LedgerEntry{}, err
	}
	return entry, nil
}

// Refund reverses one complete payment. Both records and their ledger entries
// remain immutable. The returned payment status is derived from this record.
type Refund struct {
	ShiftID                                                    string
	ID, OrderID, PaymentID, IdempotencyKey, RecordedBy, Reason string
	Amount                                                     Money
	Method                                                     PaymentMethod
	RefundedAt                                                 time.Time
}

func (r Refund) Validate() error {
	if r.ShiftID != "" {
		if err := identifier("refund.shift_id", r.ShiftID); err != nil {
			return err
		}
	}
	for _, v := range []struct{ field, value string }{{"refund.id", r.ID}, {"refund.order_id", r.OrderID}, {"refund.payment_id", r.PaymentID}, {"refund.idempotency_key", r.IdempotencyKey}, {"refund.recorded_by", r.RecordedBy}} {
		if err := identifier(v.field, v.value); err != nil {
			return err
		}
	}
	if r.Amount <= 0 || (r.Method != Cash && r.Method != BankTransfer && r.Method != QRIS) || r.RefundedAt.IsZero() {
		return invalid("refund", "Nominal, metode atau waktu refund tidak valid.")
	}
	return text("refund.reason", r.Reason, 250, true, false)
}

func NewRefundLedger(id string, r Refund) (LedgerEntry, error) {
	if err := r.Validate(); err != nil {
		return LedgerEntry{}, err
	}
	e := LedgerEntry{ShiftID: r.ShiftID, ID: id, OrderID: r.OrderID, RefundID: r.ID, RecordedBy: r.RecordedBy, Description: "Pengembalian pembayaran pesanan", Type: Credit, Method: r.Method, Amount: r.Amount, OccurredAt: r.RefundedAt}
	return e, e.Validate()
}
