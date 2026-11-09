package service

import (
	"context"
	"strings"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type Catalog struct {
	uow     repository.UnitOfWork
	runtime Runtime
}

func NewCatalog(uow repository.UnitOfWork, rt Runtime) *Catalog { return &Catalog{uow, rt} }

type ServiceInput struct {
	ID, Name        string
	Unit            domain.Unit
	UnitPrice       domain.Money
	Turnaround      time.Duration
	Active          bool
	DisplayOrder    int
	ExpectedVersion int64
}

func (s *Catalog) Configure(ctx context.Context, actor string, in ServiceInput) (repository.ServiceRecord, error) {
	var result repository.ServiceRecord
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, ConfigureService)
		if err != nil {
			return err
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		v := domain.Service{ID: in.ID, Name: strings.TrimSpace(in.Name), Unit: in.Unit, UnitPrice: in.UnitPrice, Turnaround: in.Turnaround, Active: in.Active, DisplayOrder: in.DisplayOrder, CreatedAt: at, UpdatedAt: at}
		version := int64(1)
		if in.ID == "" {
			if in.ExpectedVersion != 0 {
				return invalid("version", "Layanan baru tidak mempunyai versi sebelumnya.")
			}
			v.ID, err = s.runtime.id()
			if err != nil {
				return err
			}
			if err := v.Validate(); err != nil {
				return err
			}
			if err := tx.AddService(ctx, v); err != nil {
				return err
			}
		} else {
			old, err := tx.LockService(ctx, in.ID)
			if err != nil {
				return err
			}
			if in.ExpectedVersion < 1 || in.ExpectedVersion != old.Version {
				return conflict("stale_service", "version", "Versi layanan telah berubah.")
			}
			v.CreatedAt = old.Service.CreatedAt
			if at.Before(old.Service.UpdatedAt) {
				return invalid("clock", "Waktu pembaruan mendahului konfigurasi terakhir.")
			}
			if err := v.Validate(); err != nil {
				return err
			}
			if err := tx.UpdateService(ctx, v, old.Version); err != nil {
				return err
			}
			version = old.Version + 1
		}
		if err := tx.AppendServiceRevision(ctx, repository.ServiceRevision{ServiceRecord: repository.ServiceRecord{Service: v, Version: version}, ActorID: a.ID}); err != nil {
			return err
		}
		if err := audit(ctx, tx, s.runtime, a.ID, domain.ServiceConfigured, domain.ServiceEntity, v.ID, at); err != nil {
			return err
		}
		result = repository.ServiceRecord{Service: v, Version: version}
		return nil
	})
	if err != nil {
		return repository.ServiceRecord{}, err
	}
	return result, nil
}

func (s *Catalog) List(ctx context.Context, actor string, filter repository.ServiceFilter) ([]repository.ServiceRecord, error) {
	var result []repository.ServiceRecord
	err := s.uow.View(ctx, func(r repository.Readers) error {
		action := ReadOrders
		if filter.IncludeInactive {
			action = ConfigureService
		}
		if _, err := authorize(ctx, r, actor, action); err != nil {
			return err
		}
		p, err := filter.Page.Normalize()
		if err != nil {
			return err
		}
		filter.Page = p
		result, err = r.ListServices(ctx, filter)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Catalog) Get(ctx context.Context, actor, id string) (repository.ServiceRecord, error) {
	var result repository.ServiceRecord
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ConfigureService); err != nil {
			return err
		}
		var err error
		result, err = r.GetServiceRecord(ctx, id)
		return err
	})
	if err != nil {
		return repository.ServiceRecord{}, err
	}
	return result, nil
}
func (s *Catalog) History(ctx context.Context, actor, id string, page repository.Page) ([]repository.ServiceRevision, error) {
	var result []repository.ServiceRevision
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ConfigureService); err != nil {
			return err
		}
		if _, err := r.GetServiceRecord(ctx, id); err != nil {
			return err
		}
		p, err := page.Normalize()
		if err != nil {
			return err
		}
		result, err = r.ServiceHistory(ctx, id, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
