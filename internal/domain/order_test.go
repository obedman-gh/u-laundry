package domain_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"ulaundry/internal/domain"
)

var now = time.Date(2026, 11, 7, 8, 0, 0, 0, time.FixedZone("WITA", 8*60*60))

func staff(role domain.Role) domain.Staff {
	return domain.Staff{ID: "staff-1", Username: "staff.test", Name: "Staf Contoh", Role: role, Active: true, CreatedAt: now, UpdatedAt: now}
}

func service() domain.Service {
	return domain.Service{ID: "wash", Name: "Cuci Setrika", Unit: domain.PerKilogram, UnitPrice: 7000, Turnaround: 24 * time.Hour, Active: true, CreatedAt: now, UpdatedAt: now}
}

func customer(t *testing.T) domain.Customer {
	t.Helper()
	c, err := domain.NewCustomer("customer-1", "Pelanggan Contoh", "0812-3456-7890", "", now)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func input() domain.NewOrderInput {
	return domain.NewOrderInput{ID: "order-1", CreatedAt: now, Items: []domain.ItemRequest{{ServiceID: "wash", Quantity: domain.Quantity{Grams: 2500}}}}
}

func order(t *testing.T) domain.Order {
	t.Helper()
	o, err := domain.NewOrder(input(), customer(t), []domain.Service{service()}, staff(domain.Cashier))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func payment(id string, amount domain.Money, at time.Time) domain.Payment {
	return domain.Payment{ID: id, OrderID: "order-1", IdempotencyKey: "key-" + id, RecordedBy: "staff-1", Amount: amount, Method: domain.Cash, ReceivedAt: at}
}

func requireError(t *testing.T, err error, kind domain.ErrorKind, code string) {
	t.Helper()
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Kind != kind || (code != "" && typed.Code != code) || typed.Field == "" || typed.Message == "" {
		t.Fatalf("expected domain error %s/%s, got %v", kind, code, err)
	}
}

func TestTransitionTable(t *testing.T) {
	allowed := map[[2]domain.OrderStatus]bool{
		{domain.Received, domain.Processing}: true, {domain.Received, domain.Cancelled}: true,
		{domain.Processing, domain.Ready}: true, {domain.Processing, domain.Cancelled}: true,
		{domain.Ready, domain.Collected}: true,
	}
	statuses := []domain.OrderStatus{domain.Received, domain.Processing, domain.Ready, domain.Collected, domain.Cancelled}
	for _, from := range statuses {
		for _, to := range statuses {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				err := domain.CheckTransition(from, to)
				if allowed[[2]domain.OrderStatus{from, to}] {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					requireError(t, err, domain.Conflict, "invalid_transition")
				}
			})
		}
	}
	for _, pair := range [][2]domain.OrderStatus{{"", domain.Received}, {domain.Received, "unknown"}, {"Sudah Lunas", domain.Processing}} {
		requireError(t, domain.CheckTransition(pair[0], pair[1]), domain.Validation, "invalid_value")
	}
}

func TestLineCalculation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		unit     domain.Unit
		price    domain.Money
		quantity domain.Quantity
		want     domain.Money
	}{
		{"two_and_half_kg", domain.PerKilogram, 7000, domain.Quantity{Grams: 2500}, 17500},
		{"round_down", domain.PerKilogram, 7001, domain.Quantity{Grams: 100}, 700},
		{"half_up", domain.PerKilogram, 7005, domain.Quantity{Grams: 100}, 701},
		{"one_gram", domain.PerKilogram, 7000, domain.Quantity{Grams: 1}, 7},
		{"subrupiah_rounds_to_zero", domain.PerKilogram, 1, domain.Quantity{Grams: 1}, 0},
		{"pieces", domain.PerPiece, 15000, domain.Quantity{Pieces: 3}, 45000},
		{"free_service", domain.PerPiece, 0, domain.Quantity{Pieces: 1}, 0},
		{"max_price_one_kg", domain.PerKilogram, math.MaxInt64, domain.Quantity{Grams: 1000}, math.MaxInt64},
		{"fraction_avoids_intermediate_overflow", domain.PerKilogram, math.MaxInt64, domain.Quantity{Grams: 999}, (math.MaxInt64/1000)*999 + ((math.MaxInt64%1000)*999+500)/1000},
		{"large_grams_small_price", domain.PerKilogram, 1, domain.Quantity{Grams: math.MaxInt64}, math.MaxInt64/1000 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := service()
			s.Unit, s.UnitPrice = tc.unit, tc.price
			item, err := domain.NewOrderItem(s, tc.quantity)
			if err != nil || item.LineTotal != tc.want {
				t.Fatalf("total=%d err=%v want=%d", item.LineTotal, err, tc.want)
			}
			if err := item.Validate(); err != nil {
				t.Fatal(err)
			}
			item.LineTotal--
			requireError(t, item.Validate(), domain.Validation, "invalid_value")
		})
	}
}

func TestQuantityAndMoneyFailures(t *testing.T) {
	for _, quantity := range []domain.Quantity{{}, {Grams: -1}, {Pieces: -1}, {Grams: 1, Pieces: 1}, {Pieces: 1}} {
		_, err := domain.NewOrderItem(service(), quantity)
		requireError(t, err, domain.Validation, "invalid_value")
	}
	for _, tc := range []struct {
		unit  domain.Unit
		price domain.Money
		q     domain.Quantity
		code  string
	}{
		{domain.PerPiece, 1, domain.Quantity{Grams: 1}, "invalid_value"},
		{domain.PerPiece, -1, domain.Quantity{Pieces: 1}, "invalid_value"},
		{domain.PerPiece, math.MaxInt64, domain.Quantity{Pieces: 2}, "money_overflow"},
		{domain.PerKilogram, math.MaxInt64, domain.Quantity{Grams: 1001}, "money_overflow"},
	} {
		s := service()
		s.Unit, s.UnitPrice = tc.unit, tc.price
		_, err := domain.NewOrderItem(s, tc.q)
		requireError(t, err, domain.Validation, tc.code)
	}
	// Each line fits, but their subtotal does not.
	s := service()
	s.Unit, s.UnitPrice = domain.PerPiece, math.MaxInt64
	in := input()
	in.Items = []domain.ItemRequest{{ServiceID: s.ID, Quantity: domain.Quantity{Pieces: 1}}, {ServiceID: s.ID, Quantity: domain.Quantity{Pieces: 1}}}
	_, err := domain.NewOrder(in, customer(t), []domain.Service{s}, staff(domain.Owner))
	requireError(t, err, domain.Validation, "money_overflow")
}

func TestWeightAgainstExactArithmetic(t *testing.T) {
	// Independent arbitrary-precision reference for rounding and overflow edges.
	for _, price := range []domain.Money{0, 1, 499, 500, 7000, math.MaxInt64 / 2, math.MaxInt64} {
		for _, grams := range []int64{1, 500, 999, 1000, 1001, 1500, math.MaxInt64} {
			s := service()
			s.UnitPrice = price
			want := new(big.Int).Mul(big.NewInt(int64(price)), big.NewInt(grams))
			want.Add(want, big.NewInt(500)).Quo(want, big.NewInt(1000))
			item, err := domain.NewOrderItem(s, domain.Quantity{Grams: grams})
			if !want.IsInt64() {
				requireError(t, err, domain.Validation, "money_overflow")
			} else if err != nil || int64(item.LineTotal) != want.Int64() {
				t.Fatalf("price=%d grams=%d total=%d want=%s err=%v", price, grams, item.LineTotal, want, err)
			}
		}
	}
}

func TestOrderTotalsAndSnapshots(t *testing.T) {
	in := input()
	in.Discount = 500
	in.DiscountReason = "Penyesuaian layanan"
	in.Notes = "  Pisahkan warna\r\nTanpa pewangi  "
	kg := service()
	pieces := service()
	pieces.ID = "blanket"
	pieces.Name = "Selimut"
	pieces.Unit = domain.PerPiece
	pieces.UnitPrice = 20000
	pieces.Turnaround = 48 * time.Hour
	in.Items = append(in.Items, domain.ItemRequest{ServiceID: pieces.ID, Quantity: domain.Quantity{Pieces: 2}})
	catalog := []domain.Service{kg, pieces}
	o, err := domain.NewOrder(in, customer(t), catalog, staff(domain.Owner))
	if err != nil {
		t.Fatal(err)
	}
	if o.Subtotal() != 57500 || o.Total() != 57000 || o.Discount().ApprovedBy != "staff-1" || o.Status() != domain.Received || o.PaymentStatus() != domain.Unpaid || o.CreatedBy() != "staff-1" {
		t.Fatal("incorrect calculated order")
	}
	if !o.EstimatedReadyAt().Equal(now.Add(48*time.Hour)) || !o.CreatedAt().Equal(now) || !o.UpdatedAt().Equal(now) {
		t.Fatal("incorrect timestamps")
	}
	if o.Notes() != "Pisahkan warna\nTanpa pewangi" || o.Customer().Contact != "+6281234567890" {
		t.Fatal("normalization")
	}
	catalog[0].UnitPrice = 1
	in.Items[0].Quantity.Grams = 1
	items := o.Items()
	items[0].UnitPrice = 1
	items[0].LineTotal = 1
	if o.Items()[0].UnitPrice != 7000 || o.Total() != 57000 {
		t.Fatal("snapshot aliased caller data")
	}
	// Fractional lines round individually before summing, not once on the order.
	s := service()
	s.UnitPrice = 1
	in = input()
	in.Items = []domain.ItemRequest{{ServiceID: s.ID, Quantity: domain.Quantity{Grams: 500}}, {ServiceID: s.ID, Quantity: domain.Quantity{Grams: 500}}}
	o, err = domain.NewOrder(in, customer(t), []domain.Service{s}, staff(domain.Owner))
	if err != nil || o.Total() != 2 {
		t.Fatalf("per-line rounding: %d %v", o.Total(), err)
	}
}

func TestOrderValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.NewOrderInput)
		kind   domain.ErrorKind
		code   string
	}{
		{"missing_id", func(in *domain.NewOrderInput) { in.ID = "" }, domain.Validation, "invalid_value"},
		{"empty_items", func(in *domain.NewOrderInput) { in.Items = nil }, domain.Validation, "invalid_value"},
		{"too_many_items", func(in *domain.NewOrderInput) { in.Items = make([]domain.ItemRequest, 101) }, domain.Validation, "invalid_value"},
		{"missing_creation", func(in *domain.NewOrderInput) { in.CreatedAt = time.Time{} }, domain.Validation, "invalid_value"},
		{"estimate_before", func(in *domain.NewOrderInput) { in.EstimatedReadyAt = now.Add(-time.Second) }, domain.Validation, "invalid_value"},
		{"estimate_equal", func(in *domain.NewOrderInput) { in.EstimatedReadyAt = now }, domain.Validation, "invalid_value"},
		{"unknown_service", func(in *domain.NewOrderInput) { in.Items[0].ServiceID = "missing" }, domain.NotFound, "service_not_found"},
		{"notes_too_long", func(in *domain.NewOrderInput) { in.Notes = strings.Repeat("x", 1001) }, domain.Validation, "invalid_value"},
		{"notes_control", func(in *domain.NewOrderInput) { in.Notes = "instruction\x00" }, domain.Validation, "invalid_value"},
		{"negative_discount", func(in *domain.NewOrderInput) { in.Discount = -1 }, domain.Validation, "invalid_value"},
		{"discount_exceeds_total", func(in *domain.NewOrderInput) { in.Discount = 17501; in.DiscountReason = "Alasan" }, domain.Validation, "invalid_value"},
		{"discount_needs_reason", func(in *domain.NewOrderInput) { in.Discount = 1 }, domain.Validation, "invalid_value"},
		{"reason_without_discount", func(in *domain.NewOrderInput) { in.DiscountReason = "Alasan" }, domain.Validation, "invalid_value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := input()
			tc.change(&in)
			_, err := domain.NewOrder(in, customer(t), []domain.Service{service()}, staff(domain.Owner))
			requireError(t, err, tc.kind, tc.code)
		})
	}
	in := input()
	in.EstimatedReadyAt = now.Add(6 * time.Hour)
	if _, err := domain.NewOrder(in, customer(t), []domain.Service{service()}, staff(domain.Owner)); err != nil {
		t.Fatal(err)
	}
	in.Discount = 17500
	in.DiscountReason = "Kompensasi"
	free, err := domain.NewOrder(in, customer(t), []domain.Service{service()}, staff(domain.Admin))
	if err != nil || free.Total() != 0 || free.PaymentStatus() != domain.Paid {
		t.Fatalf("full discount: %v", err)
	}
	requireError(t, free.RecordPayment(payment("p1", 1, now)), domain.Conflict, "overpayment")
	_, err = domain.NewOrder(in, customer(t), []domain.Service{service()}, staff(domain.Cashier))
	requireError(t, err, domain.Forbidden, "discount_approval_required")
	in = input()
	s := service()
	s.Active = false
	_, err = domain.NewOrder(in, customer(t), []domain.Service{s}, staff(domain.Owner))
	requireError(t, err, domain.Conflict, "service_inactive")
	s = service()
	_, err = domain.NewOrder(in, customer(t), []domain.Service{s, s}, staff(domain.Owner))
	requireError(t, err, domain.Conflict, "duplicate_service")
	a := staff(domain.Owner)
	a.Active = false
	_, err = domain.NewOrder(in, customer(t), []domain.Service{s}, a)
	requireError(t, err, domain.Forbidden, "staff_inactive")
	c := customer(t)
	c.Name = ""
	_, err = domain.NewOrder(in, c, []domain.Service{s}, staff(domain.Owner))
	requireError(t, err, domain.Validation, "invalid_value")
}

func TestPaymentAndOperationalWorkflow(t *testing.T) {
	o := order(t)
	if err := o.RecordPayment(payment("p1", 5000, now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if o.Status() != domain.Received || o.PaymentStatus() != domain.PartiallyPaid || o.Paid() != 5000 || o.Balance() != 12500 {
		t.Fatal("payment changed operational state")
	}
	for _, step := range []struct {
		status domain.OrderStatus
		at     time.Time
	}{{domain.Processing, now.Add(2 * time.Minute)}, {domain.Ready, now.Add(3 * time.Minute)}} {
		if err := o.Transition(step.status, step.at, ""); err != nil {
			t.Fatal(err)
		}
	}
	before := o
	requireError(t, o.Transition(domain.Collected, now.Add(4*time.Minute), ""), domain.Conflict, "payment_required")
	if !reflect.DeepEqual(o, before) {
		t.Fatal("failed pickup mutated order")
	}
	if err := o.RecordPayment(payment("p2", 12500, now.Add(5*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if o.Status() != domain.Ready || o.PaymentStatus() != domain.Paid || o.Balance() != 0 {
		t.Fatal("settlement state")
	}
	if err := o.Transition(domain.Collected, now.Add(6*time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	if !o.ReadyAt().Equal(now.Add(3*time.Minute)) || !o.CollectedAt().Equal(now.Add(6*time.Minute)) || o.Paid() != 17500 {
		t.Fatal("workflow timestamps or money changed")
	}
	requireError(t, o.RecordPayment(payment("p3", 1, now.Add(7*time.Minute))), domain.Conflict, "order_closed")
	payments := o.Payments()
	payments[0].Amount = 1
	if o.Payments()[0].Amount != 5000 {
		t.Fatal("payments getter aliases state")
	}
}

func TestRejectedPaymentsDoNotMutate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.Payment)
		kind   domain.ErrorKind
		code   string
	}{
		{"zero", func(p *domain.Payment) { p.Amount = 0 }, domain.Validation, "invalid_value"},
		{"negative", func(p *domain.Payment) { p.Amount = -1 }, domain.Validation, "invalid_value"},
		{"method", func(p *domain.Payment) { p.Method = "gateway" }, domain.Validation, "invalid_value"},
		{"missing_key", func(p *domain.Payment) { p.IdempotencyKey = "" }, domain.Validation, "invalid_value"},
		{"missing_actor", func(p *domain.Payment) { p.RecordedBy = "" }, domain.Validation, "invalid_value"},
		{"wrong_order", func(p *domain.Payment) { p.OrderID = "order-2" }, domain.Validation, "invalid_value"},
		{"missing_time", func(p *domain.Payment) { p.ReceivedAt = time.Time{} }, domain.Validation, "invalid_value"},
		{"backdated", func(p *domain.Payment) { p.ReceivedAt = now.Add(-time.Second) }, domain.Validation, "invalid_value"},
		{"overpayment", func(p *domain.Payment) { p.Amount = 17501 }, domain.Conflict, "overpayment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := order(t)
			before := o
			p := payment("p1", 5000, now)
			tc.change(&p)
			requireError(t, o.RecordPayment(p), tc.kind, tc.code)
			if !reflect.DeepEqual(o, before) {
				t.Fatal("failed payment mutated order")
			}
		})
	}
	o := order(t)
	p := payment("p1", 5000, now)
	if err := o.RecordPayment(p); err != nil {
		t.Fatal(err)
	}
	before := o
	for _, duplicate := range []domain.Payment{p, {ID: "different", OrderID: p.OrderID, IdempotencyKey: p.IdempotencyKey, RecordedBy: p.RecordedBy, Amount: 1, Method: p.Method, ReceivedAt: p.ReceivedAt}} {
		requireError(t, o.RecordPayment(duplicate), domain.Conflict, "duplicate_payment")
		if !reflect.DeepEqual(o, before) {
			t.Fatal("duplicate payment changed state")
		}
	}
	if err := o.Transition(domain.Processing, now.Add(time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	requireError(t, o.RecordPayment(payment("p2", 1, now)), domain.Validation, "invalid_value")
	// A copied order remains independent when recording another payment.
	copyOrder := o
	if err := copyOrder.RecordPayment(payment("p2", 1, now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if o.Paid() != 5000 || len(o.Payments()) != 1 {
		t.Fatal("copy shares mutable payment state")
	}
}

func TestCancellationAndChronology(t *testing.T) {
	for _, processing := range []bool{false, true} {
		t.Run(fmt.Sprint(processing), func(t *testing.T) {
			o := order(t)
			if processing {
				if err := o.Transition(domain.Processing, now, ""); err != nil {
					t.Fatal(err)
				}
			}
			before := o
			requireError(t, o.Transition(domain.Cancelled, now, " "), domain.Validation, "invalid_value")
			if !reflect.DeepEqual(o, before) {
				t.Fatal("invalid cancellation mutated order")
			}
			if err := o.Transition(domain.Cancelled, now.Add(time.Minute), "  Permintaan pelanggan  "); err != nil {
				t.Fatal(err)
			}
			if o.CancellationReason() != "Permintaan pelanggan" || !o.CancelledAt().Equal(now.Add(time.Minute)) || o.PaymentStatus() != domain.Unpaid {
				t.Fatal("cancellation state")
			}
			requireError(t, o.RecordPayment(payment("p1", 1, now.Add(2*time.Minute))), domain.Conflict, "order_closed")
			requireError(t, o.Transition(domain.Processing, now.Add(2*time.Minute), ""), domain.Conflict, "invalid_transition")
		})
	}
	o := order(t)
	if err := o.RecordPayment(payment("p1", 1, now)); err != nil {
		t.Fatal(err)
	}
	requireError(t, o.Transition(domain.Cancelled, now, "Permintaan pelanggan"), domain.Conflict, "payment_exists")
	for _, at := range []time.Time{{}, now.Add(-time.Second)} {
		requireError(t, o.Transition(domain.Processing, at, ""), domain.Validation, "invalid_value")
	}
	requireError(t, o.Transition(domain.Processing, now, "Alasan yang tidak sesuai"), domain.Validation, "invalid_value")
	var zero domain.Order
	requireError(t, zero.Transition(domain.Processing, now, ""), domain.Validation, "invalid_value")
	requireError(t, zero.RecordPayment(payment("p1", 1, now)), domain.Validation, "invalid_value")
}
