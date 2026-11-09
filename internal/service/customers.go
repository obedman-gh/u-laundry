package service

import (
	"context"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type Customers struct {
	uow     repository.UnitOfWork
	runtime Runtime
}

func NewCustomers(uow repository.UnitOfWork, rt Runtime) *Customers { return &Customers{uow, rt} }

type CustomerInput struct{ Name, Contact, Address, Notes string }

func (s *Customers) Create(ctx context.Context, actor string, in CustomerInput) (domain.Customer, error) {
	var result domain.Customer
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, CreateCustomer)
		if err != nil {
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
		c, err := domain.NewCustomer(id, in.Name, in.Contact, in.Notes, at)
		if err != nil {
			return err
		}
		c, err = c.Edit(in.Name, in.Contact, in.Address, in.Notes, at)
		if err != nil {
			return err
		}
		if err := tx.AddCustomer(ctx, c); err != nil {
			return err
		}
		if err := audit(ctx, tx, s.runtime, a.ID, domain.CustomerCreated, domain.CustomerEntity, c.ID, at); err != nil {
			return err
		}
		result = c
		return nil
	})
	if err != nil {
		return domain.Customer{}, err
	}
	return result, nil
}

func (s *Customers) Search(ctx context.Context, actor, term string, page repository.Page) ([]domain.Customer, error) {
	var result []domain.Customer
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadOrders); err != nil {
			return err
		}
		p, err := page.Normalize()
		if err != nil {
			return err
		}
		q, err := domain.CustomerSearch(term)
		if err != nil {
			return err
		}
		result, err = r.SearchCustomers(ctx, q, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Customers) Get(ctx context.Context, actor, id string) (repository.CustomerRecord, error) {
	var result repository.CustomerRecord
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadOrders); err != nil {
			return err
		}
		var err error
		result, err = r.GetCustomerRecord(ctx, id)
		return err
	})
	if err != nil {
		return repository.CustomerRecord{}, err
	}
	return result, nil
}

func (s *Customers) Update(ctx context.Context, actor, id string, in CustomerInput, expected int64) (repository.CustomerRecord, error) {
	var result repository.CustomerRecord
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, CreateCustomer)
		if err != nil {
			return err
		}
		r, err := tx.LockCustomer(ctx, id)
		if err != nil {
			return err
		}
		if expected < 1 || expected != r.Version {
			return conflict("stale_customer", "version", "Data pelanggan telah berubah.")
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		c, err := r.Customer.Edit(in.Name, in.Contact, in.Address, in.Notes, at)
		if err != nil {
			return err
		}
		if err := tx.UpdateCustomer(ctx, c, expected); err != nil {
			return err
		}
		if err := audit(ctx, tx, s.runtime, a.ID, domain.CustomerUpdated, domain.CustomerEntity, id, at); err != nil {
			return err
		}
		result = repository.CustomerRecord{Customer: c, Version: expected + 1}
		return nil
	})
	if err != nil {
		return repository.CustomerRecord{}, err
	}
	return result, nil
}
