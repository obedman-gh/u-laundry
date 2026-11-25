package service

import (
	"context"
	"testing"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"

	"golang.org/x/crypto/bcrypt"
)

func TestPostgresQueueSearch(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	base := postgres.New(db)
	a := domain.Staff{ID: "queue-owner", Username: "queue.owner", Name: "Owner Contoh", Role: domain.Owner, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	c, err := domain.NewCustomer("queue-customer", "Antrean %_ 08123456789", "", "", fixed)
	if err != nil {
		t.Fatal(err)
	}
	svc := domain.Service{ID: "queue-service", Name: "Cuci", Unit: domain.PerPiece, UnitPrice: 10000, Turnaround: time.Hour, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	hash, err := bcrypt.GenerateFromPassword([]byte("synthetic-queue-unused"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.WithinTransaction(ctx, func(tx repository.Tx) error {
		if err := tx.AddStaff(ctx, a, string(hash)); err != nil {
			return err
		}
		if err := tx.AddCustomer(ctx, c); err != nil {
			return err
		}
		return tx.AddService(ctx, svc)
	}); err != nil {
		t.Fatal(err)
	}
	rt := Runtime{Clock: ClockFunc(func() time.Time { return fixed.Add(time.Hour) }), IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	orders := NewOrders(base, rt)
	var id string
	for n := 0; n < 23; n++ {
		key, err := rt.IDs.New()
		if err != nil {
			t.Fatal(err)
		}
		created, err := orders.Create(ctx, a.ID, OrderInput{IdempotencyKey: key, CustomerID: c.ID, Items: []domain.ItemRequest{{ServiceID: svc.ID, Quantity: domain.Quantity{Pieces: 1}}}})
		if err != nil {
			t.Fatal(err)
		}
		id = created.Order.ID()
	}
	if _, err := orders.ChangeStatus(ctx, a.ID, id, domain.Processing, 1, "queue-progress"); err != nil {
		t.Fatal(err)
	}
	list := func(filter repository.OrderFilter) []OrderListItem {
		t.Helper()
		rows, err := orders.List(ctx, a.ID, filter)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	first := list(repository.OrderFilter{Search: "  aNtReAn %_  ", Status: domain.Received, Page: repository.Page{Limit: 20}})
	last := list(repository.OrderFilter{Search: "antrean %_", Status: domain.Received, Page: repository.Page{Limit: 20, Offset: 20}})
	if len(first) != 20 || len(last) != 2 {
		t.Fatalf("filtered pages: %d/%d", len(first), len(last))
	}
	seen := map[string]bool{}
	for _, row := range append(first, last...) {
		if seen[row.ID] || row.Status != domain.Received {
			t.Fatal("overlap or wrong status")
		}
		seen[row.ID] = true
	}
	for _, term := range []string{"08123456789", "%", "_"} {
		if len(list(repository.OrderFilter{Search: term, Page: repository.Page{Limit: 100}})) != 23 {
			t.Fatal("literal search changed", term)
		}
	}
	rows := list(repository.OrderFilter{Search: id, Status: domain.Processing})
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatal("ID/status intersection")
	}
	if len(list(repository.OrderFilter{Search: "' OR 1=1 --"})) != 0 {
		t.Fatal("SQL term was not literal")
	}
	if _, err := orders.List(ctx, a.ID, repository.OrderFilter{Search: "bad\nquery"}); err == nil {
		t.Fatal("control character accepted")
	}
}
