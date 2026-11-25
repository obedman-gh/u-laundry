package dto

import "time"

type ShiftOpenRequest struct {
	Opening *int64 `json:"opening"`
	Notes   string `json:"notes"`
}

func (v ShiftOpenRequest) Validate() error {
	if v.Opening == nil || *v.Opening < 0 || !text(v.Notes, 250, false, false) {
		return bad("Saldo awal non-negatif dan catatan maksimal 250 karakter wajib sesuai format.")
	}
	return nil
}

type ShiftCloseRequest struct {
	Physical *int64 `json:"physical"`
	Notes    string `json:"notes"`
}

func (v ShiftCloseRequest) Validate() error {
	if v.Physical == nil || *v.Physical < 0 || !text(v.Notes, 250, false, false) {
		return bad("Saldo fisik non-negatif dan catatan maksimal 250 karakter wajib sesuai format.")
	}
	return nil
}

type CashMoveRequest struct {
	Amount    int64  `json:"amount"`
	Direction string `json:"direction"`
	Notes     string `json:"notes"`
}

func (v CashMoveRequest) Validate() error {
	if v.Amount <= 0 || (v.Direction != "in" && v.Direction != "out") || !text(v.Notes, 250, true, false) {
		return bad("Mutasi kas memerlukan nominal positif, arah in/out dan alasan satu baris.")
	}
	return nil
}

type ShiftTotalsResponse struct {
	UnknownIn    int64 `json:"unknown_in"`
	UnknownOut   int64 `json:"unknown_out"`
	CashPayments int64 `json:"cash_payments"`
	CashRefunds  int64 `json:"cash_refunds"`
	CashIn       int64 `json:"cash_in"`
	CashOut      int64 `json:"cash_out"`
	BankPayments int64 `json:"bank_payments"`
	BankRefunds  int64 `json:"bank_refunds"`
	QRISPayments int64 `json:"qris_payments"`
	QRISRefunds  int64 `json:"qris_refunds"`
}
type ShiftResponse struct {
	CashierName  string              `json:"cashier_name"`
	ID           string              `json:"id"`
	CashierID    string              `json:"cashier_id"`
	Status       string              `json:"status"`
	Opening      int64               `json:"opening"`
	OpeningNotes string              `json:"opening_notes"`
	OpenedAt     time.Time           `json:"opened_at"`
	ClosedAt     *time.Time          `json:"closed_at"`
	ClosedBy     string              `json:"closed_by"`
	ClosingNotes string              `json:"closing_notes"`
	Expected     int64               `json:"expected"`
	Physical     *int64              `json:"physical"`
	Difference   *int64              `json:"difference"`
	Totals       ShiftTotalsResponse `json:"totals"`
	Replayed     bool                `json:"replayed"`
}
type ShiftDayResponse struct {
	Date       string              `json:"date"`
	Shifts     []ShiftResponse     `json:"shifts"`
	Totals     ShiftTotalsResponse `json:"totals"`
	Unassigned ShiftTotalsResponse `json:"unassigned"`
}
type ShiftEntryResponse struct {
	ID         string    `json:"id"`
	ShiftID    string    `json:"shift_id"`
	OrderID    string    `json:"order_id"`
	PaymentID  string    `json:"payment_id"`
	RefundID   string    `json:"refund_id"`
	Type       string    `json:"type"`
	Method     string    `json:"method"`
	Amount     int64     `json:"amount"`
	Notes      string    `json:"notes"`
	CashierID  string    `json:"cashier_id"`
	OccurredAt time.Time `json:"occurred_at"`
	Replayed   bool      `json:"replayed"`
}
