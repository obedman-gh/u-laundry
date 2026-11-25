package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

const ManageShift Action = "manage_shift"

type Shifts struct {
	uow     repository.UnitOfWork
	runtime Runtime
}

func NewShifts(u repository.UnitOfWork, r Runtime) *Shifts { return &Shifts{u, r} }

type ShiftView struct {
	CashierName string
	Shift       domain.CashShift
	Totals      domain.ShiftTotals
	Expected    domain.Money
	Replayed    bool
}
type ShiftOpenInput struct {
	Key, Notes string
	Opening    domain.Money
}
type ShiftCloseInput struct {
	ID, Key, Notes string
	Physical       domain.Money
}
type CashMoveInput struct {
	ID, Key, Notes string
	Direction      domain.LedgerType
	Amount         domain.Money
}
type CashMoveResult struct {
	Movement domain.CashMovement
	Replayed bool
}
type ShiftDay struct {
	Date               string
	Shifts             []ShiftView
	Totals, Unassigned domain.ShiftTotals
}

func shiftView(ctx context.Context, r repository.Readers, s domain.CashShift) (ShiftView, error) {
	t, err := r.ShiftTotals(ctx, s.ID)
	if err != nil {
		return ShiftView{}, err
	}
	expected, err := t.ExpectedCash(s.Opening)
	if err != nil {
		return ShiftView{}, err
	}
	if !s.IsOpen() && s.System != expected {
		return ShiftView{}, &repository.Error{Kind: repository.InvalidData, Operation: "closed shift reconciliation"}
	}
	staff, err := r.GetStaff(ctx, s.CashierID)
	if err != nil {
		return ShiftView{}, err
	}
	return ShiftView{CashierName: staff.Name, Shift: s, Totals: t, Expected: expected}, nil
}
func shiftAccess(a domain.Staff, s domain.CashShift, write bool) error {
	if a.ID == s.CashierID || !write && (a.Role == domain.Owner || a.Role == domain.Admin) {
		return nil
	}
	return &AccessDenied{ActorID: a.ID, Permission: ManageShift}
}
func (s *Shifts) Open(ctx context.Context, actor string, in ShiftOpenInput) (ShiftView, error) {
	in.Notes = strings.TrimSpace(in.Notes)
	var result ShiftView
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, ManageShift)
		if err != nil {
			return err
		}
		if err = domain.ValidateIdempotencyKey(in.Key); err != nil {
			return err
		}
		if err = tx.LockShiftOwner(ctx, a.ID); err != nil {
			return err
		}
		existing, err := tx.FindShiftByOpenKey(ctx, a.ID, in.Key)
		if err == nil {
			if existing.Opening != in.Opening || existing.OpeningNotes != in.Notes {
				return conflict("idempotency_conflict", "key", "Kunci buka kas telah dipakai untuk data berbeda.")
			}
			result, err = shiftView(ctx, tx, existing)
			result.Replayed = true
			return err
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		if _, err = tx.ActiveShift(ctx, a.ID); err == nil {
			return conflict("active_shift_exists", "shift", "Tutup shift aktif sebelum membuka shift baru.")
		} else if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		id, err := s.runtime.id()
		if err != nil {
			return err
		}
		v := domain.CashShift{ID: id, CashierID: a.ID, OpenKey: in.Key, Opening: in.Opening, OpeningNotes: in.Notes, OpenedAt: at}
		if err = v.Validate(); err != nil {
			return err
		}
		if err = tx.AddShift(ctx, v); err != nil {
			return err
		}
		if err = audit(ctx, tx, s.runtime, a.ID, domain.ShiftOpened, domain.ShiftEntity, id, at); err != nil {
			return err
		}
		result, err = shiftView(ctx, tx, v)
		return err
	})
	if err != nil {
		return ShiftView{}, err
	}
	return result, nil
}
func (s *Shifts) Close(ctx context.Context, actor string, in ShiftCloseInput) (ShiftView, error) {
	in.Notes = strings.TrimSpace(in.Notes)
	var result ShiftView
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, ManageShift)
		if err != nil {
			return err
		}
		if err = domain.ValidateIdempotencyKey(in.Key); err != nil {
			return err
		}
		v, err := tx.LockShift(ctx, in.ID)
		if err != nil {
			return err
		}
		if err = shiftAccess(a, v, true); err != nil {
			return err
		}
		if !v.IsOpen() {
			if v.CloseKey != in.Key || v.Physical != in.Physical || v.ClosingNotes != in.Notes {
				return conflict("shift_closed", "shift", "Shift sudah ditutup dengan data berbeda.")
			}
			result, err = shiftView(ctx, tx, v)
			result.Replayed = true
			return err
		}
		t, err := tx.ShiftTotals(ctx, v.ID)
		if err != nil {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		if err = v.Close(t, in.Physical, a.ID, in.Key, in.Notes, at); err != nil {
			return err
		}
		if err = tx.CloseShift(ctx, v); err != nil {
			return err
		}
		if err = audit(ctx, tx, s.runtime, a.ID, domain.ShiftClosed, domain.ShiftEntity, v.ID, at); err != nil {
			return err
		}
		result = ShiftView{CashierName: a.Name, Shift: v, Totals: t, Expected: v.System}
		return nil
	})
	if err != nil {
		return ShiftView{}, err
	}
	return result, nil
}
func (s *Shifts) Move(ctx context.Context, actor string, in CashMoveInput) (CashMoveResult, error) {
	in.Notes = strings.TrimSpace(in.Notes)
	var result CashMoveResult
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, ManageShift)
		if err != nil {
			return err
		}
		if err = domain.ValidateIdempotencyKey(in.Key); err != nil {
			return err
		}
		v, err := tx.LockShift(ctx, in.ID)
		if err != nil {
			return err
		}
		if err = shiftAccess(a, v, true); err != nil {
			return err
		}
		existing, err := tx.FindCashMovement(ctx, v.ID, in.Key)
		if err == nil {
			if existing.Entry.Amount != in.Amount || existing.Entry.Type != in.Direction || existing.Entry.Description != in.Notes {
				return conflict("idempotency_conflict", "key", "Kunci mutasi kas telah dipakai untuk data berbeda.")
			}
			result = CashMoveResult{existing, true}
			return nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		t, err := tx.ShiftTotals(ctx, v.ID)
		if err != nil {
			return err
		}
		if err = v.CheckPosting(t, domain.Cash, in.Direction, in.Amount); err != nil {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		id, err := s.runtime.id()
		if err != nil {
			return err
		}
		m := domain.CashMovement{Key: in.Key, Entry: domain.LedgerEntry{ID: id, ShiftID: v.ID, RecordedBy: a.ID, Description: in.Notes, Amount: in.Amount, Type: in.Direction, Method: domain.Cash, OccurredAt: at}}
		if err = m.Validate(); err != nil {
			return err
		}
		if err = tx.AddCashMovement(ctx, m); err != nil {
			return err
		}
		if err = audit(ctx, tx, s.runtime, a.ID, domain.ShiftCashMoved, domain.ShiftEntity, v.ID, at); err != nil {
			return err
		}
		result = CashMoveResult{Movement: m}
		return nil
	})
	if err != nil {
		return CashMoveResult{}, err
	}
	return result, nil
}
func (s *Shifts) Current(ctx context.Context, actor string) (*ShiftView, error) {
	var result *ShiftView
	err := s.uow.View(ctx, func(r repository.Readers) error {
		a, err := authorize(ctx, r, actor, ManageShift)
		if err != nil {
			return err
		}
		v, err := r.ActiveShift(ctx, a.ID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		view, err := shiftView(ctx, r, v)
		if err == nil {
			result = &view
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Shifts) Get(ctx context.Context, actor, id string) (ShiftView, error) {
	var result ShiftView
	err := s.uow.View(ctx, func(r repository.Readers) error {
		a, err := authorize(ctx, r, actor, ManageShift)
		if err != nil {
			return err
		}
		v, err := r.GetShift(ctx, id)
		if err != nil {
			return err
		}
		if err = shiftAccess(a, v, false); err != nil {
			return err
		}
		result, err = shiftView(ctx, r, v)
		return err
	})
	if err != nil {
		return ShiftView{}, err
	}
	return result, nil
}
func (s *Shifts) Entries(ctx context.Context, actor, id string, p repository.Page) ([]domain.LedgerEntry, error) {
	var result []domain.LedgerEntry
	err := s.uow.View(ctx, func(r repository.Readers) error {
		a, err := authorize(ctx, r, actor, ManageShift)
		if err != nil {
			return err
		}
		v, err := r.GetShift(ctx, id)
		if err != nil {
			return err
		}
		if err = shiftAccess(a, v, false); err != nil {
			return err
		}
		result, err = r.ShiftEntries(ctx, id, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Shifts) Daily(ctx context.Context, actor, date string) (ShiftDay, error) {
	var result ShiftDay
	err := s.uow.View(ctx, func(r repository.Readers) error {
		a, err := authorize(ctx, r, actor, ManageShift)
		if err != nil {
			return err
		}
		zone := time.FixedZone("WITA", 8*60*60)
		if date == "" {
			at, err := s.runtime.now()
			if err != nil {
				return err
			}
			date = at.In(zone).Format("2006-01-02")
		}
		from, err := time.ParseInLocation("2006-01-02", date, zone)
		if err != nil || from.Year() < 1 {
			return invalid("date", "Tanggal laporan harus YYYY-MM-DD.")
		}
		filter := ""
		if a.Role == domain.Cashier {
			filter = a.ID
		}
		rows, err := r.ShiftsForDay(ctx, from, from.Add(24*time.Hour), filter)
		if err != nil {
			return err
		}
		result = ShiftDay{Date: date, Shifts: make([]ShiftView, 0, len(rows))}
		for _, v := range rows {
			view, err := shiftView(ctx, r, v)
			if err != nil {
				return err
			}
			result.Shifts = append(result.Shifts, view)
		}
		result.Totals, err = r.DailyShiftTotals(ctx, from, from.Add(24*time.Hour), filter, false)
		if err != nil {
			return err
		}
		result.Unassigned, err = r.DailyShiftTotals(ctx, from, from.Add(24*time.Hour), filter, true)
		return err
	})
	if err != nil {
		return ShiftDay{}, err
	}
	return result, nil
}

func postingShift(ctx context.Context, tx repository.Tx, actor string, method domain.PaymentMethod, direction domain.LedgerType, amount domain.Money) (string, error) {
	id, err := tx.LockPostingShift(ctx, actor, method)
	if err != nil || id == "" {
		return id, err
	}
	v, err := tx.GetShift(ctx, id)
	if err != nil {
		return "", err
	}
	totals, err := tx.ShiftTotals(ctx, id)
	if err != nil {
		return "", err
	}
	return id, v.CheckPosting(totals, method, direction, amount)
}
