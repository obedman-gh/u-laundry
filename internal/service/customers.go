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

type CustomerInput struct{ Name, Contact, Notes string }

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
