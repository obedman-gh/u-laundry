package domain

import "time"

type PaymentStatus string

const (
	Unpaid        PaymentStatus = "unpaid"
	PartiallyPaid PaymentStatus = "partially_paid"
	Paid          PaymentStatus = "paid"
)

type PaymentMethod string

const (
	Cash         PaymentMethod = "cash"
	BankTransfer PaymentMethod = "bank_transfer"
	QRIS         PaymentMethod = "qris"
)

// Payment is a recorded positive receipt, not a pending gateway transaction.
// ReceivedAt and RecordedBy are supplied by the trusted server use case.
type Payment struct {
	ID, OrderID, IdempotencyKey, RecordedBy string
	Amount                                  Money
	Method                                  PaymentMethod
	ReceivedAt                              time.Time
}

func (p Payment) Validate() error {
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
	ID, OrderID, PaymentID, RecordedBy, Description string
	Type                                            LedgerType
	Amount                                          Money
	OccurredAt                                      time.Time
}

func (e LedgerEntry) Validate() error {
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
	return nil
}

// NewPaymentLedger copies the payment's authoritative amount and references.
// It does not persist anything or itself guarantee one posting per payment.
func NewPaymentLedger(id string, payment Payment) (LedgerEntry, error) {
	if err := payment.Validate(); err != nil {
		return LedgerEntry{}, err
	}
	entry := LedgerEntry{ID: id, OrderID: payment.OrderID, PaymentID: payment.ID, RecordedBy: payment.RecordedBy, Description: "Penerimaan pembayaran pesanan", Type: Debit, Amount: payment.Amount, OccurredAt: payment.ReceivedAt}
	if err := entry.Validate(); err != nil {
		return LedgerEntry{}, err
	}
	return entry, nil
}
