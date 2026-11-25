package domain

import (
	"math"
	"testing"
	"time"
)

func TestCashShiftReconciliation(t *testing.T) {
	at := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	base := CashShift{ID: "shift-1", CashierID: "cashier-1", OpenKey: "open-1", Opening: 100000, OpenedAt: at}
	totals := ShiftTotals{CashPayments: 30000, CashRefunds: 10000, CashIn: 5000, CashOut: 15000, BankPayments: 500000, QRISPayments: 200000}
	if got, err := totals.ExpectedCash(base.Opening); err != nil || got != 110000 {
		t.Fatal(got, err)
	}
	for _, physical := range []Money{109000, 110000, 111000} {
		s := base
		if err := s.Close(totals, physical, s.CashierID, "close-1", "Hasil hitung kas", at.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if s.System != 110000 || s.Difference != physical-110000 || s.ClosedBy != base.CashierID {
			t.Fatal(s)
		}
		if err := s.Close(totals, physical, s.CashierID, "close-1", "", at); err == nil {
			t.Fatal("closed shift reopened")
		}
	}
	for _, test := range []struct {
		name         string
		physical     Money
		actor, notes string
		at           time.Time
	}{
		{"difference_requires_note", 109000, base.CashierID, "", at},
		{"negative_physical", -1, base.CashierID, "Alasan", at},
		{"other_actor", 110000, "another-cashier", "", at},
		{"past_close", 110000, base.CashierID, "", at.Add(-time.Second)},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := base
			if err := s.Close(totals, test.physical, test.actor, "close", test.notes, test.at); err == nil {
				t.Fatal("invalid close accepted")
			}
			if s != base {
				t.Fatal("failed close mutated state")
			}
		})
	}
	if err := base.CheckPosting(totals, Cash, Credit, 110001); err == nil {
		t.Fatal("cash shortage accepted")
	}
	if err := base.CheckPosting(totals, QRIS, Credit, 900000); err != nil {
		t.Fatal("noncash changed drawer", err)
	}
	for _, v := range []ShiftTotals{{CashPayments: -1}, {CashRefunds: 1}, {CashPayments: Money(math.MaxInt64), CashIn: 1}, {BankRefunds: -1}} {
		if _, err := v.ExpectedCash(0); err == nil {
			t.Fatal("invalid totals accepted", v)
		}
	}
}
