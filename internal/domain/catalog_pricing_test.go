package domain_test

import (
	"testing"
	"time"
	"ulaundry/internal/domain"
)

func TestCatalogDisplayOrderValidation(t *testing.T) {
	for _, value := range []int{0, 10, 1000000} {
		s := service()
		s.DisplayOrder = value
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []int{-1, 1000001} {
		s := service()
		s.DisplayOrder = value
		requireError(t, s.Validate(), domain.Validation, "")
	}
}
func TestCatalogSnapshotPriceAndUnitChanges(t *testing.T) {
	s := service()
	s.UnitPrice = 7005
	in := input()
	in.Items[0].Quantity.Grams = 100
	in.Discount = 1
	in.DiscountReason = "Penyesuaian"
	old, err := domain.NewOrder(in, customer(t), []domain.Service{s}, staff(domain.Owner))
	if err != nil {
		t.Fatal(err)
	}
	if old.Subtotal() != 701 || old.Total() != 700 {
		t.Fatal("half-up and discount")
	}
	s.Name = "Layanan Baru"
	s.Unit = domain.PerPiece
	s.UnitPrice = 15000
	s.Turnaround = 48 * time.Hour
	s.DisplayOrder = 3
	in.ID = "order-new"
	in.Items[0].Quantity = domain.Quantity{Pieces: 2}
	in.Discount = 1000
	newOrder, err := domain.NewOrder(in, customer(t), []domain.Service{s}, staff(domain.Admin))
	if err != nil {
		t.Fatal(err)
	}
	if newOrder.Subtotal() != 30000 || newOrder.Total() != 29000 || newOrder.Items()[0].Unit != domain.PerPiece {
		t.Fatal("new price/unit")
	}
	s.Active = false
	_, err = domain.NewOrder(in, customer(t), []domain.Service{s}, staff(domain.Owner))
	requireError(t, err, domain.Conflict, "service_inactive")
	restored, err := domain.RestoreOrder(old.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	line := restored.Items()[0]
	if line.ServiceName != "Cuci Setrika" || line.Unit != domain.PerKilogram || line.UnitPrice != 7005 || line.Quantity.Grams != 100 || line.LineTotal != 701 || restored.Discount().Amount != 1 || restored.Total() != 700 || !restored.EstimatedReadyAt().Equal(now.Add(24*time.Hour)) {
		t.Fatal("old financial snapshot changed")
	}
}
func TestCatalogMinimumTotalZeroPolicy(t *testing.T) {
	in := input()
	in.Discount = 17500
	in.DiscountReason = "Diskon penuh"
	o, err := domain.NewOrder(in, customer(t), []domain.Service{service()}, staff(domain.Owner))
	if err != nil || o.Total() != 0 || o.PaymentStatus() != domain.Paid {
		t.Fatal("approved zero minimum", err)
	}
	in.Discount++
	_, err = domain.NewOrder(in, customer(t), []domain.Service{service()}, staff(domain.Owner))
	requireError(t, err, domain.Validation, "")
	in.Discount = -1
	_, err = domain.NewOrder(in, customer(t), []domain.Service{service()}, staff(domain.Owner))
	requireError(t, err, domain.Validation, "")
}
