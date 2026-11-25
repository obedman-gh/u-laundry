package service

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/testutil"
)

type reportSnapshotWork struct {
	repository.UnitOfWork
	write func()
}
type reportSnapshotReader struct {
	repository.Readers
	write func()
}

func (w reportSnapshotWork) View(ctx context.Context, fn func(repository.Readers) error) error {
	return w.UnitOfWork.View(ctx, func(r repository.Readers) error { return fn(reportSnapshotReader{r, w.write}) })
}
func (r reportSnapshotReader) DashboardMetrics(ctx context.Context, w repository.ReportWindow) (repository.DashboardMetrics, error) {
	// authorize has already established the read snapshot; commit another
	// connection's payment before any reporting query is issued.
	r.write()
	return r.Readers.DashboardMetrics(ctx, w)
}

func TestPostgresOperationalReports(t *testing.T) {
	db := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := migrate.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	base := postgres.New(db)
	zone := time.FixedZone("WITA", 8*60*60)
	from := time.Date(2026, 11, 24, 0, 0, 0, 0, zone).UTC()
	to := from.Add(24 * time.Hour)
	now := from.Add(-time.Hour)
	owner := domain.Staff{ID: "report-owner", Username: "report.owner", Name: "Owner Contoh", Role: domain.Owner, Active: true, CreatedAt: now, UpdatedAt: now}
	cashier := owner
	cashier.ID = "report-cashier"
	cashier.Username = "report.cashier"
	cashier.Role = domain.Cashier
	admin := owner
	admin.ID = "report-admin"
	admin.Username = "report.admin"
	admin.Role = domain.Admin
	c, err := domain.NewCustomer("report-customer", "=Pelanggan Contoh", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	svc := domain.Service{ID: "report-service", Name: "Cuci Contoh", Unit: domain.PerPiece, UnitPrice: 10000, Turnaround: time.Hour, Active: true, CreatedAt: now, UpdatedAt: now}
	hash, err := bcrypt.GenerateFromPassword([]byte("synthetic-report-unused"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.WithinTransaction(ctx, func(tx repository.Tx) error {
		for _, a := range []domain.Staff{owner, cashier, admin} {
			if err := tx.AddStaff(ctx, a, string(hash)); err != nil {
				return err
			}
		}
		if err := tx.AddCustomer(ctx, c); err != nil {
			return err
		}
		return tx.AddService(ctx, svc)
	}); err != nil {
		t.Fatal(err)
	}
	rt := Runtime{Clock: ClockFunc(func() time.Time { return now }), IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	reports := NewReports(base, rt)
	period := ReportPeriod{From: from, To: to}
	t.Run("empty", func(t *testing.T) {
		v, err := reports.Dashboard(ctx, owner.ID, period)
		if err != nil || v.Metrics.Money.Payments != 0 || len(v.Metrics.Methods) != 4 || v.Metrics.Popular == nil {
			t.Fatal(v, err)
		}
	})
	shifts := NewShifts(base, rt)
	shift, err := shifts.Open(ctx, owner.ID, ShiftOpenInput{Key: "report-open", Opening: 100000})
	if err != nil {
		t.Fatal(err)
	}
	orders, payments := NewOrders(base, rt), NewPayments(base, rt)
	create := func(key string, at time.Time, discount domain.Money) string {
		t.Helper()
		now = at
		in := OrderInput{IdempotencyKey: key, CustomerID: c.ID, Items: []domain.ItemRequest{{ServiceID: svc.ID, Quantity: domain.Quantity{Pieces: 1}}}, Discount: discount}
		if discount > 0 {
			in.DiscountReason = "Penyesuaian"
		}
		v, err := orders.Create(ctx, owner.ID, in)
		if err != nil {
			t.Fatal(key, err)
		}
		return v.Order.ID()
	}
	pay := func(id, key, actor string, amount domain.Money, method domain.PaymentMethod, at time.Time) domain.Payment {
		t.Helper()
		now = at
		v, err := payments.Record(ctx, actor, PaymentInput{OrderID: id, IdempotencyKey: key, Amount: amount, Method: method})
		if err != nil {
			t.Fatal(key, err)
		}
		return v.Payment
	}
	old := create("before", from.Add(-time.Microsecond), 0)
	pay(old, "before-payment", owner.ID, 3000, domain.Cash, from.Add(-time.Microsecond))
	pay(old, "start-old-payment", owner.ID, 2000, domain.Cash, from)
	start := create("start", from, 1000)
	pay(start, "start-payment", owner.ID, 4000, domain.Cash, from)
	pay(start, "qris-payment", cashier.ID, 5000, domain.QRIS, from.Add(time.Hour))
	stored, err := orders.Get(ctx, owner.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	now = from.Add(time.Hour)
	stored, err = orders.ChangeStatus(ctx, owner.ID, start, domain.Processing, stored.Version, "start-processing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = orders.ChangeStatus(ctx, owner.ID, start, domain.Ready, stored.Version, "start-ready"); err != nil {
		t.Fatal(err)
	}
	refunded := create("refunded", from.Add(10*time.Minute), 0)
	p := pay(refunded, "bank-payment", cashier.ID, 10000, domain.BankTransfer, from.Add(20*time.Minute))
	unpaid := create("unpaid", from.Add(2*time.Hour), 0)
	processing := create("processing", from.Add(time.Hour), 0)
	stored, err = orders.Get(ctx, owner.ID, processing)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = orders.ChangeStatus(ctx, owner.ID, processing, domain.Processing, stored.Version, "late-processing"); err != nil {
		t.Fatal(err)
	}
	now = from.Add(3 * time.Hour)
	if _, err := payments.Refund(ctx, owner.ID, RefundInput{OrderID: refunded, PaymentID: p.ID, IdempotencyKey: "refund", Reason: "Pembatalan"}); err != nil {
		t.Fatal(err)
	}
	stored, err = orders.Get(ctx, owner.ID, refunded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.Cancel(ctx, owner.ID, refunded, stored.Version, "Dibatalkan pelanggan", "cancel-refunded"); err != nil {
		t.Fatal(err)
	}
	for _, move := range []struct {
		key       string
		direction domain.LedgerType
		amount    domain.Money
	}{{"cash-in", domain.Debit, 700}, {"cash-out", domain.Credit, 300}} {
		if _, err := shifts.Move(ctx, owner.ID, CashMoveInput{ID: shift.Shift.ID, Key: move.key, Direction: move.direction, Amount: move.amount, Notes: "Mutasi kas"}); err != nil {
			t.Fatal(err)
		}
	}
	now = from.Add(4 * time.Hour)
	// Opening also includes the 3000 receipt immediately before the period.
	if _, err := shifts.Close(ctx, owner.ID, ShiftCloseInput{ID: shift.Shift.ID, Key: "close", Physical: 109350, Notes: "Selisih hitung"}); err != nil {
		t.Fatal(err)
	}
	if err := base.WithinTransaction(ctx, func(tx repository.Tx) error {
		return tx.RecordExpense(ctx, domain.LedgerEntry{ID: "report-expense", Type: domain.Credit, Amount: 1200, Description: "=SUM(1,2)", RecordedBy: owner.ID, OccurredAt: from.Add(5 * time.Hour)})
	}); err != nil {
		t.Fatal(err)
	}
	end := create("end", to, 0)
	pay(end, "end-payment", cashier.ID, 7000, domain.QRIS, to)
	now = to.Add(15 * time.Minute)
	if _, err := shifts.Open(ctx, owner.ID, ShiftOpenInput{Key: "second-open", Opening: 0}); err != nil {
		t.Fatal(err)
	}
	now = to.Add(time.Hour)

	t.Run("numbers-and-boundaries", func(t *testing.T) {
		v, err := reports.Dashboard(ctx, owner.ID, period)
		if err != nil {
			t.Fatal(err)
		}
		m := v.Metrics
		if m.Orders != (repository.OrderMetrics{Created: 4, Received: 3, Processing: 1, Ready: 1, Overdue: 3, ReceivableOrders: 4, Receivable: 28000}) {
			t.Fatal("order totals", m.Orders)
		}
		if m.Money != (repository.MoneyMetrics{Payments: 21000, Refunds: 10000, Net: 11000, Expenses: 1200, CashIn: 700, CashOut: 300, UnassignedPayments: 15000}) {
			t.Fatal("money totals", m.Money)
		}
		if m.Shifts != (repository.ShiftMetrics{Opened: 0, Closed: 1, Active: 1, System: 109400, Physical: 109350, Difference: -50}) {
			t.Fatal("shift totals", m.Shifts)
		}
		if len(m.Popular) != 1 || m.Popular[0].Orders != 3 || m.Popular[0].Pieces != 3 || m.Popular[0].Gross != 30000 {
			t.Fatal("popular", m.Popular)
		}
		methods := map[string]repository.MethodMetrics{}
		for _, x := range m.Methods {
			methods[x.Method] = x
		}
		if methods["cash"].Payments != 6000 || methods["qris"].Payments != 5000 || methods["bank_transfer"].Payments != 10000 || methods["bank_transfer"].Refunds != 10000 {
			t.Fatal(methods)
		}
		day, err := reports.Dashboard(ctx, admin.ID, ReportPeriod{})
		if err != nil || !day.Window.From.Equal(to) || day.Metrics.Orders.Created != 1 || day.Metrics.Money.Payments != 7000 {
			t.Fatal("WITA default", day, err)
		}
	})
	t.Run("detail-pagination-filters", func(t *testing.T) {
		seen := map[string]bool{}
		for offset := 0; offset < 8; offset += 2 {
			v, err := reports.LedgerDetails(ctx, owner.ID, period, repository.Page{Limit: 2, Offset: offset})
			if err != nil {
				t.Fatal(err)
			}
			if len(v.Rows.Items) != 2 || v.Rows.HasMore != (offset < 6) {
				t.Fatal("ledger page", v.Rows)
			}
			for _, x := range v.Rows.Items {
				if seen[x.ID] {
					t.Fatal("duplicate page entry")
				}
				seen[x.ID] = true
			}
		}
		v, err := reports.LedgerDetails(ctx, owner.ID, period, repository.Page{Limit: 2, Offset: 8})
		if err != nil || v.Rows.Items == nil || len(v.Rows.Items) != 0 || v.Rows.HasMore {
			t.Fatal("empty ledger", v, err)
		}
		for _, tc := range []struct {
			kind   string
			status domain.OrderStatus
			count  int
		}{{"created", "", 4}, {"created", domain.Cancelled, 1}, {"receivable", "", 4}, {"overdue", "", 3}, {"overdue", domain.Ready, 0}} {
			v, err := reports.OrderDetails(ctx, admin.ID, period, repository.ReportOrderFilter{Kind: tc.kind, Status: tc.status})
			if err != nil || len(v.Rows.Items) != tc.count {
				t.Fatal(tc, v, err)
			}
		}
		first, err := reports.OrderDetails(ctx, owner.ID, period, repository.ReportOrderFilter{Kind: "created", Page: repository.Page{Limit: 1}})
		if err != nil || !first.Rows.HasMore || first.Rows.Items[0].ID != unpaid {
			t.Fatal(first, err)
		}
	})
	t.Run("authorization-and-audit-rollback", func(t *testing.T) {
		_, err := reports.Dashboard(ctx, cashier.ID, period)
		requireDomain(t, err, domain.Forbidden, "")
		_, err = reports.OrderDetails(ctx, cashier.ID, period, repository.ReportOrderFilter{})
		requireDomain(t, err, domain.Forbidden, "")
		_, err = reports.LedgerDetails(ctx, cashier.ID, period, repository.Page{})
		requireDomain(t, err, domain.Forbidden, "")
		var before, after int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM public.audit_events WHERE action='report_generated'`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		v, err := NewReports(refundFailureWork{base, "audit"}, rt).Dashboard(ctx, owner.ID, period)
		if !errors.Is(err, failure) || v.ID != "" || v.Metrics.Orders.Created != 0 {
			t.Fatal("audit failure leaked", v, err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM public.audit_events WHERE action='report_generated'`).Scan(&after); err != nil || before != after {
			t.Fatal("audit rollback", before, after, err)
		}
	})
	t.Run("consistent-snapshot", func(t *testing.T) {
		work := reportSnapshotWork{UnitOfWork: base, write: func() { pay(unpaid, "concurrent", cashier.ID, 1000, domain.QRIS, now) }}
		v, err := NewReports(work, rt).Dashboard(ctx, owner.ID, period)
		if err != nil || v.Metrics.Orders.Receivable != 28000 {
			t.Fatal("mixed snapshot", v, err)
		}
		next, err := reports.Dashboard(ctx, owner.ID, period)
		if err != nil || next.Metrics.Orders.Receivable != 27000 {
			t.Fatal("write missing", next, err)
		}
	})
	t.Run("query-error-and-overflow", func(t *testing.T) {
		cancelled, stop := context.WithCancel(ctx)
		stop()
		v, err := reports.Dashboard(cancelled, owner.ID, period)
		if err == nil || v.ID != "" {
			t.Fatal("cancel ignored", err)
		}
		// Two individually valid int64 expenses must fail the aggregate rather
		// than wrap or return a misleading financial success.
		for i := 0; i < 2; i++ {
			if err := base.WithinTransaction(ctx, func(tx repository.Tx) error {
				return tx.RecordExpense(ctx, domain.LedgerEntry{ID: fmt.Sprintf("large-expense-%d", i), Type: domain.Credit, Amount: domain.Money(1 << 62), Description: "Overflow fixture", RecordedBy: owner.ID, OccurredAt: from})
			}); err != nil {
				t.Fatal(err)
			}
		}
		v, err = reports.Dashboard(ctx, owner.ID, period)
		if err == nil || v.ID != "" {
			t.Fatal("aggregate overflow returned success", v, err)
		}
	})
}
