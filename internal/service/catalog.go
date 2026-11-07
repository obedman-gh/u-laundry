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
		v := domain.Service{ID: in.ID, Name: strings.TrimSpace(in.Name), Unit: in.Unit, UnitPrice: in.UnitPrice, Turnaround: in.Turnaround, Active: in.Active, CreatedAt: at, UpdatedAt: at}
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
