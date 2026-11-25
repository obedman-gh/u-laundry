package domain

import "time"

// ShiftTotals separates drawer cash from non-cash receipts.
type ShiftTotals struct {
	UnknownIn, UnknownOut                                Money
	CashPayments, CashRefunds, CashIn, CashOut           Money
	BankPayments, BankRefunds, QRISPayments, QRISRefunds Money
}

func (v ShiftTotals) ExpectedCash(opening Money) (Money, error) {
	in, err := addMoney(opening, v.CashPayments)
	if err != nil {
		return 0, err
	}
	in, err = addMoney(in, v.CashIn)
	if err != nil {
		return 0, err
	}
	out, err := addMoney(v.CashRefunds, v.CashOut)
	if err != nil {
		return 0, err
	}
	for _, m := range []Money{v.BankPayments, v.BankRefunds, v.QRISPayments, v.QRISRefunds, v.UnknownIn, v.UnknownOut} {
		if err := m.Validate("totals"); err != nil {
			return 0, err
		}
	}
	if out > in {
		return 0, conflict("insufficient_cash", "cash", "Saldo kas sistem tidak mencukupi.")
	}
	return in - out, nil
}

type CashShift struct {
	ID, CashierID, OpenKey, OpeningNotes string
	Opening                              Money
	OpenedAt                             time.Time
	ClosedAt                             time.Time
	ClosedBy, CloseKey, ClosingNotes     string
	System, Physical, Difference         Money
}

func (s CashShift) IsOpen() bool { return s.ClosedAt.IsZero() }
func (s CashShift) CheckPosting(t ShiftTotals, method PaymentMethod, direction LedgerType, amount Money) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if !s.IsOpen() {
		return conflict("shift_closed", "shift", "Shift sudah ditutup.")
	}
	if amount <= 0 || (direction != Debit && direction != Credit) {
		return invalid("posting", "Nominal positif dan arah kas yang sah wajib diisi.")
	}
	if method != Cash {
		return nil
	}
	var err error
	if direction == Debit {
		t.CashIn, err = addMoney(t.CashIn, amount)
	} else {
		t.CashOut, err = addMoney(t.CashOut, amount)
	}
	if err != nil {
		return err
	}
	_, err = t.ExpectedCash(s.Opening)
	return err
}
func (s CashShift) Validate() error {
	for _, v := range []struct{ field, value string }{{"shift.id", s.ID}, {"shift.cashier_id", s.CashierID}, {"shift.open_key", s.OpenKey}} {
		if err := identifier(v.field, v.value); err != nil {
			return err
		}
	}
	if err := s.Opening.Validate("opening"); err != nil {
		return err
	}
	if s.OpenedAt.IsZero() {
		return invalid("opened_at", "Waktu buka kas wajib diisi.")
	}
	if err := text("opening_notes", s.OpeningNotes, 250, false, false); err != nil {
		return err
	}
	if s.IsOpen() {
		if s.ClosedBy != "" || s.CloseKey != "" || s.ClosingNotes != "" || s.System != 0 || s.Physical != 0 || s.Difference != 0 {
			return invalid("shift", "Data penutupan belum boleh diisi.")
		}
		return nil
	}
	if s.ClosedAt.Before(s.OpenedAt) || s.ClosedBy != s.CashierID {
		return invalid("closed_at", "Penutupan kas tidak sesuai pemilik atau waktu buka.")
	}
	if err := identifier("close_key", s.CloseKey); err != nil {
		return err
	}
	if s.System < 0 || s.Physical < 0 || s.Difference != s.Physical-s.System {
		return invalid("reconciliation", "Rekonsiliasi kas tidak valid.")
	}
	return text("closing_notes", s.ClosingNotes, 250, s.Difference != 0, false)
}

func (s *CashShift) Close(t ShiftTotals, physical Money, actor, key, notes string, at time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if !s.IsOpen() {
		return conflict("shift_closed", "shift", "Shift sudah ditutup.")
	}
	expected, err := t.ExpectedCash(s.Opening)
	if err != nil {
		return err
	}
	next := *s
	next.ClosedAt, next.ClosedBy, next.CloseKey, next.ClosingNotes = at, actor, key, notes
	next.System, next.Physical, next.Difference = expected, physical, physical-expected
	if err := next.Validate(); err != nil {
		return err
	}
	*s = next
	return nil
}

type CashMovement struct {
	Entry LedgerEntry
	Key   string
}

func (m CashMovement) Validate() error {
	if err := identifier("movement.key", m.Key); err != nil {
		return err
	}
	if err := m.Entry.Validate(); err != nil {
		return err
	}
	if m.Entry.ShiftID == "" || m.Entry.Method != Cash || m.Entry.OrderID != "" || m.Entry.PaymentID != "" || m.Entry.RefundID != "" {
		return invalid("movement", "Pergerakan kas wajib terkait shift tanpa relasi pesanan.")
	}
	return nil
}
