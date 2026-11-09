package service

import (
	"context"
	"errors"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (t *fakeTx) GetCustomerRecord(ctx context.Context, id string) (repository.CustomerRecord, error) {
	c, err := t.GetCustomer(ctx, id)
	version := t.state.customerVersions[id]
	if version == 0 {
		version = 1
	}
	return repository.CustomerRecord{Customer: c, Version: version}, err
}
func (t *fakeTx) LockCustomer(ctx context.Context, id string) (repository.CustomerRecord, error) {
	return t.GetCustomerRecord(ctx, id)
}
func (t *fakeTx) UpdateCustomer(_ context.Context, c domain.Customer, _ int64) error {
	if err := t.call("update_customer"); err != nil {
		return err
	}
	t.state.customers[c.ID] = c
	if t.state.customerVersions == nil {
		t.state.customerVersions = map[string]int64{}
	}
	t.state.customerVersions[c.ID]++
	if t.state.customerVersions[c.ID] == 1 {
		t.state.customerVersions[c.ID] = 2
	}
	return nil
}
func (t *fakeTx) SearchCustomers(_ context.Context, term string, p repository.Page) ([]domain.Customer, error) {
	if err := t.call("search_customer"); err != nil {
		return nil, err
	}
	if term != "+6281234567890" || p.Limit != 20 {
		return nil, failure
	}
	return []domain.Customer{t.state.customers["customer-1"]}, nil
}

func TestCustomerManagementAuthorizationAndAtomicity(t *testing.T) {
	ctx := context.Background()
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
		u, rt := fixture(t, role)
		s := NewCustomers(u, rt)
		items, err := s.Search(ctx, "actor-1", "0812-3456-7890", repository.Page{})
		if err != nil || len(items) != 1 {
			t.Fatal("search", err)
		}
		r, err := s.Get(ctx, "actor-1", "customer-1")
		if err != nil || r.Version != 1 {
			t.Fatal("get", err)
		}
		updated, err := s.Update(ctx, "actor-1", r.Customer.ID, CustomerInput{Name: "Updated", Address: "Viola"}, 1)
		if err != nil || updated.Version != 2 || updated.Customer.Address != "Viola" || updated.Customer.Contact != "" || u.state.audits[0].Action != domain.CustomerUpdated {
			t.Fatal("update", err)
		}
		_, err = s.Update(ctx, "actor-1", r.Customer.ID, CustomerInput{Name: "Stale"}, 1)
		requireDomain(t, err, domain.Conflict, "")
		u.failAt = "audit"
		_, err = s.Update(ctx, "actor-1", r.Customer.ID, CustomerInput{Name: "Must roll back"}, 2)
		if !errors.Is(err, failure) || u.state.customers[r.Customer.ID].Name != "Updated" {
			t.Fatal("customer/audit atomicity", err)
		}
		a := u.state.staff["actor-1"]
		a.Active = false
		u.state.staff[a.ID] = a
		_, err = s.Search(ctx, a.ID, "", repository.Page{})
		requireDomain(t, err, domain.Forbidden, "")
		_, err = s.Get(ctx, a.ID, r.Customer.ID)
		requireDomain(t, err, domain.Forbidden, "")
		_, err = s.Update(ctx, a.ID, r.Customer.ID, CustomerInput{Name: "Denied"}, 1)
		requireDomain(t, err, domain.Forbidden, "")
	}
}

// Expiry and uniqueness are exercised against PostgreSQL, not mirrored in this fake.
func (t *fakeTx) ReplaceTracking(_ context.Context, id string, hash [32]byte, _ time.Time, expected int64) error {
	if err := t.call("replace_tracking"); err != nil {
		return err
	}
	if _, ok := t.state.tracking[hash]; ok {
		return &repository.Error{Kind: repository.Conflict}
	}
	for h, order := range t.state.tracking {
		if order == id {
			delete(t.state.tracking, h)
		}
	}
	t.state.tracking[hash] = id
	r := t.state.orders[id]
	r.Version++
	t.state.orders[id] = r
	return nil
}
func TestTrackingRotationAuthorizationAndRollback(t *testing.T) {
	ctx := context.Background()
	u, rt := fixture(t, domain.Cashier)
	s := NewOrders(u, rt)
	rotated, err := s.RotateTracking(ctx, "actor-1", "order-1", 1)
	if err != nil || rotated.Version != 2 || !rotated.ExpiresAt.Equal(fixed.Add(domain.TrackingLifetime)) || u.state.audits[0].Action != domain.OrderTrackingRotated {
		t.Fatal("rotation", err)
	}
	_, err = s.RotateTracking(ctx, "actor-1", "order-1", 1)
	requireDomain(t, err, domain.Conflict, "")
	_, err = s.RotateTracking(ctx, "actor-1", "order-1", 2)
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatal("same generated token accepted", err)
	}
	u, rt = fixture(t, domain.Cashier)
	u.failAt = "audit"
	result, err := NewOrders(u, rt).RotateTracking(ctx, "actor-1", "order-1", 1)
	if !errors.Is(err, failure) || result.Token != "" || len(u.state.tracking) != 0 || u.state.orders["order-1"].Version != 1 {
		t.Fatal("rotation survived failed audit", err)
	}
	a := u.state.staff["actor-1"]
	a.Active = false
	u.state.staff[a.ID] = a
	_, err = NewOrders(u, rt).RotateTracking(ctx, a.ID, "order-1", 1)
	requireDomain(t, err, domain.Forbidden, "")
}
