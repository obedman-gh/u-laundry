package service

import (
	"context"
	"time"
	"ulaundry/internal/repository"
)

type AuditTrail struct {
	uow     repository.UnitOfWork
	runtime Runtime
}

func NewAuditTrail(uow repository.UnitOfWork, rt Runtime) *AuditTrail { return &AuditTrail{uow, rt} }

type AuditHistory struct {
	Filter repository.AuditFilter
	Page   repository.AuditPage
}

func (s *AuditTrail) List(ctx context.Context, actor string, f repository.AuditFilter) (AuditHistory, error) {
	var result AuditHistory
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadAudit); err != nil {
			return err
		}
		if f.From.IsZero() && f.To.IsZero() {
			at, err := s.runtime.now()
			if err != nil {
				return err
			}
			local := at.In(time.FixedZone("WITA", 8*60*60))
			end := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, local.Location())
			f.From, f.To = end.AddDate(0, 0, -30).UTC(), end.UTC()
		}
		f.From, f.To = f.From.UTC().Truncate(time.Microsecond), f.To.UTC().Truncate(time.Microsecond)
		if err := f.Validate(); err != nil {
			return err
		}
		p, err := f.Page.Normalize()
		if err != nil {
			return err
		}
		f.Page = p
		rows, err := r.ListAudit(ctx, f)
		if err != nil {
			return err
		}
		result = AuditHistory{Filter: f, Page: rows}
		return nil
	})
	if err != nil {
		return AuditHistory{}, err
	}
	return result, nil
}
