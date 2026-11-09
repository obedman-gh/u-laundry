package postgres

import (
	"context"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/repository"
	"ulaundry/internal/testutil"
)

func TestPostgresTrackingRepositoryConstraints(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	s := New(db)
	at := testAt
	expires := at.Add(domain.TrackingLifetime)
	actor := domain.Staff{ID: "actor", Username: "staff.test", Name: "Synthetic", Role: domain.Cashier, Active: true, CreatedAt: at, UpdatedAt: at}
	c, err := domain.NewCustomer("customer", "Synthetic", "", "", at)
	if err != nil {
		t.Fatal(err)
	}
	svc := domain.Service{ID: "wash", Name: "Cuci", Unit: domain.PerPiece, UnitPrice: 1000, Turnaround: time.Hour, Active: true, CreatedAt: at, UpdatedAt: at}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("synthetic-unused"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WithinTransaction(ctx, func(tx repository.Tx) error {
		if err := tx.AddStaff(ctx, actor, string(passwordHash)); err != nil {
			return err
		}
		if err := tx.AddCustomer(ctx, c); err != nil {
			return err
		}
		if err := tx.AddService(ctx, svc); err != nil {
			return err
		}
		for _, id := range []string{"order-a", "order-b"} {
			o, err := domain.NewOrder(domain.NewOrderInput{ID: id, CreatedAt: at, Items: []domain.ItemRequest{{ServiceID: svc.ID, Quantity: domain.Quantity{Pieces: 1}}}}, c, []domain.Service{svc}, actor)
			if err != nil {
				return err
			}
			if err := tx.CreateOrder(ctx, o); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	hash := [32]byte{7}
	if err = s.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddTracking(ctx, "order-a", hash, expires) }); err != nil {
		t.Fatal(err)
	}
	err = s.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddTracking(ctx, "order-b", hash, expires) })
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatal("hash uniqueness not enforced", err)
	}
	for _, invalid := range [][]byte{make([]byte, 31), make([]byte, 33), nil} {
		if _, err = db.ExecContext(ctx, `INSERT INTO order_tracking_tokens(order_id,token_hash,expires_at)VALUES('order-b',$1,$2)`, invalid, expires); err == nil {
			t.Fatal("invalid hash length accepted")
		}
	}
	if err = s.View(ctx, func(r repository.Readers) error {
		if _, err := r.GetTracking(ctx, hash, expires.Add(-time.Microsecond)); err != nil {
			return err
		}
		if _, err := r.GetTracking(ctx, hash, expires); !errors.Is(err, repository.ErrNotFound) {
			t.Fatal("expiry boundary", err)
		}
		if _, err := r.GetTracking(ctx, [32]byte{8}, at); !errors.Is(err, repository.ErrNotFound) {
			t.Fatal("unknown token", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
