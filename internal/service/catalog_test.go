package service

import (
	"context"
	"errors"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (t *fakeTx) AppendServiceRevision(_ context.Context, r repository.ServiceRevision) error {
	if err := t.call("service_revision"); err != nil {
		return err
	}
	r.Source = "staff"
	t.state.revisions = append(t.state.revisions, r)
	return nil
}
func (t *fakeTx) GetServiceRecord(_ context.Context, id string) (repository.ServiceRecord, error) {
	v, ok := t.state.services[id]
	if !ok {
		return v, missing()
	}
	return v, nil
}
func (t *fakeTx) ListServices(_ context.Context, filter repository.ServiceFilter) ([]repository.ServiceRecord, error) {
	if err := t.call("list_services"); err != nil {
		return nil, err
	}
	result := []repository.ServiceRecord{}
	for _, r := range t.state.services {
		if filter.IncludeInactive || r.Service.Active {
			result = append(result, r)
		}
	}
	return result, nil
}
func (t *fakeTx) ServiceHistory(_ context.Context, id string, _ repository.Page) ([]repository.ServiceRevision, error) {
	if err := t.call("service_history"); err != nil {
		return nil, err
	}
	return t.state.revisions, nil
}

func TestCatalogConfigureHistoryAndPermissions(t *testing.T) {
	ctx := context.Background()
	for _, role := range []domain.Role{domain.Owner, domain.Admin} {
		u, rt := fixture(t, role)
		s := NewCatalog(u, rt)
		r, err := s.Configure(ctx, "actor-1", ServiceInput{ID: "wash", Name: "Nama Baru", Unit: domain.PerPiece, UnitPrice: 9000, Turnaround: 48 * time.Hour, Active: false, DisplayOrder: 7, ExpectedVersion: 1})
		if err != nil || r.Version != 2 || len(u.state.revisions) != 1 || u.state.revisions[0].ActorID != "actor-1" || u.state.revisions[0].Service.DisplayOrder != 7 {
			t.Fatal("history", err)
		}
		if u.state.orders["order-1"].Order.Total() != 17500 || u.state.orders["order-1"].Order.Items()[0].Unit != domain.PerKilogram {
			t.Fatal("old order recalculated")
		}
		found, err := s.List(ctx, "actor-1", repository.ServiceFilter{IncludeInactive: true})
		if err != nil || len(found) != 1 {
			t.Fatal("manage list", err)
		}
		history, err := s.History(ctx, "actor-1", "wash", repository.Page{})
		if err != nil || len(history) != 1 {
			t.Fatal("read history", err)
		}
		_, err = s.Configure(ctx, "actor-1", ServiceInput{ID: "wash", Name: "Stale", Unit: domain.PerKilogram, Turnaround: time.Hour, ExpectedVersion: 1})
		requireDomain(t, err, domain.Conflict, "")
	}
	u, rt := fixture(t, domain.Cashier)
	s := NewCatalog(u, rt)
	if _, err := s.List(ctx, "actor-1", repository.ServiceFilter{}); err != nil {
		t.Fatal("cashier active catalog", err)
	}
	_, err := s.List(ctx, "actor-1", repository.ServiceFilter{IncludeInactive: true})
	requireDomain(t, err, domain.Forbidden, "")
	_, err = s.Get(ctx, "actor-1", "wash")
	requireDomain(t, err, domain.Forbidden, "")
	_, err = s.History(ctx, "actor-1", "wash", repository.Page{})
	requireDomain(t, err, domain.Forbidden, "")
	_, err = s.Configure(ctx, "actor-1", ServiceInput{})
	requireDomain(t, err, domain.Forbidden, "")
}
func TestCatalogRevisionAndAuditRollback(t *testing.T) {
	for _, failAt := range []string{"service_revision", "audit"} {
		u, rt := fixture(t, domain.Owner)
		u.failAt = failAt
		r, err := NewCatalog(u, rt).Configure(context.Background(), "actor-1", ServiceInput{ID: "wash", Name: "Changed", Unit: domain.PerKilogram, UnitPrice: 8000, Turnaround: time.Hour, Active: true, ExpectedVersion: 1})
		if !errors.Is(err, failure) || r.Version != 0 || u.state.services["wash"].Service.UnitPrice != 7000 || len(u.state.revisions) != 0 {
			t.Fatal("partial catalog update", err)
		}
	}
}
