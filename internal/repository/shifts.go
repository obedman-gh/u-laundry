package repository

import (
	"context"
	"time"
	"ulaundry/internal/domain"
)

type ShiftQueries interface {
	GetShift(context.Context, string) (domain.CashShift, error)
	ActiveShift(context.Context, string) (domain.CashShift, error)
	FindShiftByOpenKey(context.Context, string, string) (domain.CashShift, error)
	ShiftTotals(context.Context, string) (domain.ShiftTotals, error)
	ShiftsForDay(context.Context, time.Time, time.Time, string) ([]domain.CashShift, error)
	DailyShiftTotals(context.Context, time.Time, time.Time, string, bool) (domain.ShiftTotals, error)
	ShiftEntries(context.Context, string, Page) ([]domain.LedgerEntry, error)
	FindCashMovement(context.Context, string, string) (domain.CashMovement, error)
}
type ShiftCommands interface {
	LockShiftOwner(context.Context, string) error
	LockShift(context.Context, string) (domain.CashShift, error)
	LockPostingShift(context.Context, string, domain.PaymentMethod) (string, error)
	AddShift(context.Context, domain.CashShift) error
	CloseShift(context.Context, domain.CashShift) error
	AddCashMovement(context.Context, domain.CashMovement) error
}
