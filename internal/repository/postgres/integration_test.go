package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/repository"
	"ulaundry/internal/testutil"
)

var testAt = time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)

func TestPostgresRepositories(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	hash, err := bcrypt.GenerateFromPassword([]byte("synthetic-test-only"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	actor := domain.Staff{ID: "staff-1", Username: "staff.test", Name: "Staf Contoh", Role: domain.Owner, Active: true, CreatedAt: testAt, UpdatedAt: testAt}
	customer, err := domain.NewCustomer("customer-1", "Pelanggan 50%VIP", "081234567890", "Catatan contoh", testAt)
	if err != nil {
		t.Fatal(err)
	}
	service := domain.Service{ID: "service-1", Name: "Cuci Setrika", Unit: domain.PerKilogram, UnitPrice: 7000, Turnaround: 24 * time.Hour, Active: true, CreatedAt: testAt, UpdatedAt: testAt}
	setup := func(t *testing.T) (*sql.DB, *Store) {
		t.Helper()
		db := testutil.Postgres(t)
		if err := migrate.Up(ctx, db); err != nil {
			t.Fatal(err)
		}
		store := New(db)
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
			if err := tx.AddStaff(ctx, actor, string(hash)); err != nil {
				return err
			}
			if err := tx.AddCustomer(ctx, customer); err != nil {
				return err
			}
			return tx.AddService(ctx, service)
		}); err != nil {
			t.Fatal(err)
		}
		return db, store
	}
	newOrder := func(t *testing.T, id string, c domain.Customer, s domain.Service) domain.Order {
		t.Helper()
		o, err := domain.NewOrder(domain.NewOrderInput{ID: id, CreatedAt: testAt, Items: []domain.ItemRequest{{ServiceID: s.ID, Quantity: domain.Quantity{Grams: 2500}}}}, c, []domain.Service{s}, actor)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	create := func(t *testing.T, store *Store, o domain.Order) {
		t.Helper()
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.CreateOrder(ctx, o) }); err != nil {
			t.Fatal(err)
		}
	}
	get := func(t *testing.T, store *Store, id string) repository.OrderRecord {
		t.Helper()
		var record repository.OrderRecord
		if err := store.View(ctx, func(r repository.Readers) error { var err error; record, err = r.GetOrder(ctx, id); return err }); err != nil {
			t.Fatal(err)
		}
		return record
	}
	count := func(t *testing.T, db *sql.DB, table string) int {
		t.Helper()
		var n int
		// Table names here are fixed test literals, never application input.
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM public.`+pq.QuoteIdentifier(table)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	requireKind := func(t *testing.T, err error, kind repository.ErrorKind) {
		t.Helper()
		var typed *repository.Error
		var pg *pq.Error
		if !errors.As(err, &typed) || typed.Kind != kind || errors.As(err, &pg) {
			t.Fatalf("expected safe repository %s, got %v", kind, err)
		}
	}
	payment := func(id, orderID, key string, amount domain.Money) domain.Payment {
		return domain.Payment{ID: id, OrderID: orderID, IdempotencyKey: key, RecordedBy: actor.ID, Amount: amount, Method: domain.Cash, ReceivedAt: testAt.Add(time.Minute)}
	}

	t.Run("create_read_snapshots_and_reference_repositories", func(t *testing.T) {
		db, store := setup(t)
		o := newOrder(t, "order-1", customer, service)
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
			if err := tx.CreateOrder(ctx, o); err != nil {
				return err
			}
			return tx.AppendAudit(ctx, domain.AuditEvent{ID: "audit-1", ActorID: actor.ID, Action: domain.OrderCreated, Entity: domain.OrderEntity, EntityID: o.ID(), OccurredAt: testAt})
		}); err != nil {
			t.Fatal(err)
		}
		r := get(t, store, o.ID())
		if r.Version != 1 || r.Order.Total() != 17500 || r.Order.Status() != domain.Received || r.Order.PaymentStatus() != domain.Unpaid || !reflect.DeepEqual(r.Order.Items(), o.Items()) || !r.Order.CreatedAt().Equal(testAt) {
			t.Fatal("order round trip failed")
		}
		// Updating the live catalog/customer must not rewrite an accepted snapshot.
		if _, err := db.ExecContext(ctx, `UPDATE public.services SET unit_price=9999; UPDATE public.customers SET name='Nama Baru'`); err != nil {
			t.Fatal(err)
		}
		r = get(t, store, o.ID())
		if r.Order.Items()[0].UnitPrice != 7000 || r.Order.Customer().Name != customer.Name {
			t.Fatal("historical snapshot changed")
		}
		if err := store.View(ctx, func(r repository.Readers) error {
			c, err := r.GetCustomer(ctx, customer.ID)
			if err != nil {
				return err
			}
			if c.Name != "Nama Baru" {
				t.Fatal("customer read")
			}
			s, err := r.GetService(ctx, service.ID)
			if err != nil {
				return err
			}
			if s.UnitPrice != 9999 {
				t.Fatal("service read")
			}
			staff, err := r.GetStaff(ctx, actor.ID)
			if err != nil {
				return err
			}
			if staff.Username != actor.Username {
				t.Fatal("staff read")
			}
			credentials, err := r.FindCredentials(ctx, actor.Username)
			if err != nil {
				return err
			}
			if credentials.PasswordHash != string(hash) {
				t.Fatal("credentials round trip")
			}
			events, err := r.AuditForEntity(ctx, domain.OrderEntity, o.ID(), repository.Page{})
			if err != nil {
				return err
			}
			if len(events) != 1 || events[0].Action != domain.OrderCreated {
				t.Fatal("audit read")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("status_customer_filters_and_stable_pagination", func(t *testing.T) {
		_, store := setup(t)
		other := customer
		other.ID = "customer-2"
		other.Name = "Pelanggan Lain"
		inactive := service
		inactive.ID = "inactive"
		inactive.Active = false
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
			if err := tx.AddCustomer(ctx, other); err != nil {
				return err
			}
			return tx.AddService(ctx, inactive)
		}); err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= 5; i++ {
			c := customer
			if i == 4 {
				c = other
			}
			o := newOrder(t, fmt.Sprintf("order-%02d", i), c, service)
			create(t, store, o)
		}
		for _, change := range []struct {
			id     string
			status domain.OrderStatus
			reason string
		}{{"order-02", domain.Processing, ""}, {"order-03", domain.Processing, ""}, {"order-03", domain.Ready, ""}, {"order-05", domain.Cancelled, "Permintaan pelanggan"}} {
			if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
				r, err := tx.LockOrder(ctx, change.id)
				if err != nil {
					return err
				}
				if err := r.Order.Transition(change.status, testAt, change.reason); err != nil {
					return err
				}
				return tx.SaveOrderProgress(ctx, r.Order, r.Version)
			}); err != nil {
				t.Fatal(err)
			}
		}
		for _, tc := range []struct {
			filter repository.OrderFilter
			ids    []string
		}{
			{repository.OrderFilter{Status: domain.Processing}, []string{"order-02"}},
			{repository.OrderFilter{Status: domain.Ready}, []string{"order-03"}},
			{repository.OrderFilter{Status: domain.Received, Page: repository.Page{Limit: 1}}, []string{"order-04"}},
			{repository.OrderFilter{Status: domain.Received, Page: repository.Page{Limit: 1, Offset: 1}}, []string{"order-01"}},
			{repository.OrderFilter{CustomerID: other.ID}, []string{"order-04"}},
			{repository.OrderFilter{Status: domain.Processing, CustomerID: other.ID}, []string{}},
			{repository.OrderFilter{CustomerID: `' OR 1=1 --`}, []string{}},
			{repository.OrderFilter{Page: repository.Page{Offset: 99}}, []string{}},
		} {
			if err := store.View(ctx, func(r repository.Readers) error {
				items, err := r.ListOrders(ctx, tc.filter)
				if err != nil {
					return err
				}
				if items == nil {
					t.Fatal("nil empty page")
				}
				ids := make([]string, 0, len(items))
				for _, item := range items {
					ids = append(ids, item.ID)
				}
				if !reflect.DeepEqual(ids, tc.ids) {
					t.Fatalf("filter %+v got %v want %v", tc.filter, ids, tc.ids)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.View(ctx, func(r repository.Readers) error {
			matches, err := r.SearchCustomers(ctx, "%", repository.Page{})
			if err != nil {
				return err
			}
			if len(matches) != 1 || matches[0].ID != customer.ID {
				t.Fatal("search wildcard not literal")
			}
			empty, err := r.SearchCustomers(ctx, `' OR 1=1 --`, repository.Page{})
			if err != nil {
				return err
			}
			if empty == nil || len(empty) != 0 {
				t.Fatal("search injection or nil result")
			}
			services, err := r.ListActiveServices(ctx, repository.Page{})
			if err != nil {
				return err
			}
			if len(services) != 1 || services[0].ID != service.ID {
				t.Fatal("active filter")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for _, f := range []repository.OrderFilter{{Status: `received' OR 1=1 --`}, {Page: repository.Page{Limit: 101}}, {Page: repository.Page{Limit: -1}}, {Page: repository.Page{Offset: -1}}} {
			err := store.View(ctx, func(r repository.Readers) error { _, err := r.ListOrders(ctx, f); return err })
			var typed *domain.Error
			if !errors.As(err, &typed) || typed.Kind != domain.Validation {
				t.Fatal("invalid filter accepted", err)
			}
		}
	})

	t.Run("not_found_and_typed_conflicts", func(t *testing.T) {
		_, store := setup(t)
		lookups := []func(repository.Readers) error{
			func(r repository.Readers) error { _, err := r.GetCustomer(ctx, "missing"); return err }, func(r repository.Readers) error { _, err := r.GetService(ctx, "missing"); return err },
			func(r repository.Readers) error { _, err := r.GetOrder(ctx, "missing"); return err }, func(r repository.Readers) error { _, err := r.GetStaff(ctx, "missing"); return err },
			func(r repository.Readers) error { _, err := r.FindCredentials(ctx, "missing"); return err }, func(r repository.Readers) error { _, err := r.FindPaymentByKey(ctx, "missing", "key"); return err },
		}
		for _, lookup := range lookups {
			err := store.View(ctx, lookup)
			requireKind(t, err, repository.NotFound)
			if !errors.Is(err, repository.ErrNotFound) {
				t.Fatal("not found identity lost")
			}
		}
		err := store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddCustomer(ctx, customer) })
		requireKind(t, err, repository.Conflict)
		if strings.Contains(err.Error(), customer.Name) || strings.Contains(err.Error(), customer.ID) {
			t.Fatal("driver data disclosed")
		}
		duplicateUser := actor
		duplicateUser.ID = "staff-2"
		err = store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddStaff(ctx, duplicateUser, string(hash)) })
		requireKind(t, err, repository.Conflict)
		duplicateUser.Username = "other.test"
		err = store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddStaff(ctx, duplicateUser, "private-plain-password") })
		var invalidHash *domain.Error
		if !errors.As(err, &invalidHash) || invalidHash.Code != "invalid_password_hash" || strings.Contains(err.Error(), "private-plain") {
			t.Fatal("invalid hash accepted or exposed", err)
		}
		o := newOrder(t, "order-1", customer, service)
		create(t, store, o)
		r := get(t, store, o.ID())
		if err := r.Order.Transition(domain.Processing, testAt, ""); err != nil {
			t.Fatal(err)
		}
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.SaveOrderProgress(ctx, r.Order, r.Version) }); err != nil {
			t.Fatal(err)
		}
		err = store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.SaveOrderProgress(ctx, r.Order, r.Version) })
		requireKind(t, err, repository.Conflict)
		if get(t, store, o.ID()).Version != 2 {
			t.Fatal("stale write applied")
		}
	})

	t.Run("cross_repository_rollback_including_payment_ledger_audit", func(t *testing.T) {
		db, store := setup(t)
		c := customer
		c.ID = "rollback-customer"
		o := newOrder(t, "rollback-order", c, service)
		rollbackCause := errors.New("synthetic use case failure")
		err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
			if err := tx.AddCustomer(ctx, c); err != nil {
				return err
			}
			if err := tx.CreateOrder(ctx, o); err != nil {
				return err
			}
			if _, err := tx.RecordPayment(ctx, payment("rollback-payment", o.ID(), "rollback-key", 5000), "rollback-ledger"); err != nil {
				return err
			}
			if err := tx.AppendAudit(ctx, domain.AuditEvent{ID: "rollback-audit", ActorID: actor.ID, Action: domain.PaymentRecorded, Entity: domain.PaymentEntity, EntityID: "rollback-payment", OccurredAt: testAt.Add(time.Minute)}); err != nil {
				return err
			}
			return rollbackCause
		})
		if !errors.Is(err, rollbackCause) {
			t.Fatal(err)
		}
		if count(t, db, "orders") != 0 || count(t, db, "order_items") != 0 || count(t, db, "payments") != 0 || count(t, db, "ledger_entries") != 0 || count(t, db, "audit_events") != 0 || count(t, db, "customers") != 1 {
			t.Fatal("partial transaction survived")
		}
		err = store.View(ctx, func(r repository.Readers) error { _, err := r.GetOrder(ctx, o.ID()); return err })
		requireKind(t, err, repository.NotFound)
		// A late SQL constraint failure must also roll back previous inserts.
		err = store.WithinTransaction(ctx, func(tx repository.Tx) error {
			if err := tx.CreateOrder(ctx, newOrder(t, "late-failure", customer, service)); err != nil {
				return err
			}
			return tx.AddCustomer(ctx, customer)
		})
		requireKind(t, err, repository.Conflict)
		if count(t, db, "orders") != 0 {
			t.Fatal("SQL failure retained earlier order")
		}
	})

	t.Run("create_item_failure_rolls_back_header", func(t *testing.T) {
		db, store := setup(t)
		s := service
		s.ID = "not-stored-service"
		err := store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.CreateOrder(ctx, newOrder(t, "invalid-order", customer, s)) })
		requireKind(t, err, repository.Constraint)
		if count(t, db, "orders") != 0 || count(t, db, "order_items") != 0 {
			t.Fatal("header survived failed item")
		}
	})

	t.Run("bigint_amounts_are_not_truncated", func(t *testing.T) {
		_, store := setup(t)
		s := service
		s.ID = "large-price"
		s.UnitPrice = 3000000000
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.AddService(ctx, s) }); err != nil {
			t.Fatal(err)
		}
		o := newOrder(t, "big-order", customer, s)
		create(t, store, o)
		if got := get(t, store, o.ID()).Order.Total(); got != 7500000000 {
			t.Fatalf("bigint truncated: %d", got)
		}
	})

	t.Run("payment_replay_and_constraint_protection", func(t *testing.T) {
		db, store := setup(t)
		o := newOrder(t, "order-1", customer, service)
		create(t, store, o)
		p := payment("p1", o.ID(), "key-1", 5000)
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error { _, err := tx.RecordPayment(ctx, p, "ledger-1"); return err }); err != nil {
			t.Fatal(err)
		}
		retry := p
		retry.ID = "regenerated-id"
		retry.ReceivedAt = testAt.Add(2 * time.Minute)
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
			result, err := tx.RecordPayment(ctx, retry, "regenerated-ledger")
			if err == nil && (!result.Replayed || result.Payment.ID != p.ID) {
				t.Fatal("replay not stable")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if count(t, db, "payments") != 1 || count(t, db, "ledger_entries") != 1 || get(t, store, o.ID()).Version != 2 {
			t.Fatal("replay posted again")
		}
		retry.Amount++
		err := store.WithinTransaction(ctx, func(tx repository.Tx) error { _, err := tx.RecordPayment(ctx, retry, "unused-ledger"); return err })
		requireKind(t, err, repository.Conflict)
		// A unique ledger collision happens after the new payment insert. Both
		// that payment and the order version must roll back with the transaction.
		err = store.WithinTransaction(ctx, func(tx repository.Tx) error {
			_, err := tx.RecordPayment(ctx, payment("p2", o.ID(), "key-2", 1000), "ledger-1")
			return err
		})
		requireKind(t, err, repository.Conflict)
		if count(t, db, "payments") != 1 || get(t, store, o.ID()).Order.Paid() != 5000 || get(t, store, o.ID()).Version != 2 {
			t.Fatal("failed ledger retained payment/version")
		}
		if err := store.View(ctx, func(r repository.Readers) error {
			ledger, err := r.LedgerForOrder(ctx, o.ID(), repository.Page{})
			if err != nil {
				return err
			}
			if len(ledger) != 1 || ledger[0].Amount != p.Amount || ledger[0].PaymentID != p.ID {
				t.Fatal("ledger mismatch")
			}
			items, err := r.ListOrders(ctx, repository.OrderFilter{})
			if err != nil {
				return err
			}
			if len(items) != 1 || items[0].Paid != 5000 || items[0].Status != domain.Received {
				t.Fatal("financial/operational state mixed")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for _, query := range []string{
			`UPDATE public.services SET unit_price=-1`,
			`UPDATE public.orders SET status='unknown'`,
			`UPDATE public.order_items SET line_total=line_total+1`,
			`UPDATE public.payments SET amount=0`,
			`UPDATE public.ledger_entries SET type='OTHER'`,
		} {
			_, err := db.ExecContext(ctx, query)
			var pg *pq.Error
			if !errors.As(err, &pg) || pg.Code != "23514" {
				t.Fatalf("constraint not enforced: %v", err)
			}
		}
		_, err = db.ExecContext(ctx, `UPDATE public.ledger_entries SET amount=amount+1`)
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "23503" {
			t.Fatal("payment/ledger amount FK missing", err)
		}
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
			return tx.AppendAudit(ctx, domain.AuditEvent{ID: "audit-1", ActorID: actor.ID, Action: domain.OrderCreated, Entity: domain.OrderEntity, EntityID: o.ID(), OccurredAt: testAt})
		}); err != nil {
			t.Fatal(err)
		}
		_, err = db.ExecContext(ctx, `DELETE FROM public.audit_events`)
		if !errors.As(err, &pg) || pg.Code != "23514" {
			t.Fatal("audit mutation allowed", err)
		}
		expense := domain.LedgerEntry{ID: "expense-1", Type: domain.Credit, Amount: 45000, Description: "Pembelian sabun", RecordedBy: actor.ID, OccurredAt: testAt}
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error { return tx.RecordExpense(ctx, expense) }); err != nil {
			t.Fatal(err)
		}
		var unlinked bool
		if err := db.QueryRowContext(ctx, `SELECT order_id IS NULL AND payment_id IS NULL FROM public.ledger_entries WHERE id=$1`, expense.ID).Scan(&unlinked); err != nil || !unlinked {
			t.Fatal("expense relation invalid", err)
		}
	})

	t.Run("parallel_same_key_posts_once", func(t *testing.T) {
		db, store := setup(t)
		o := newOrder(t, "order-1", customer, service)
		create(t, store, o)
		type outcome struct {
			replayed bool
			err      error
		}
		results := make(chan outcome, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func(i int) {
				<-start
				var replayed bool
				err := store.WithinTransaction(ctx, func(tx repository.Tx) error {
					p := payment(fmt.Sprintf("p%d", i), o.ID(), "same-key", 17500)
					result, err := tx.RecordPayment(ctx, p, fmt.Sprintf("ledger-%d", i))
					if err != nil {
						return err
					}
					replayed = result.Replayed
					if !result.Replayed {
						return tx.AppendAudit(ctx, domain.AuditEvent{ID: "payment-audit", ActorID: actor.ID, Action: domain.PaymentRecorded, Entity: domain.PaymentEntity, EntityID: result.Payment.ID, OccurredAt: result.Payment.ReceivedAt})
					}
					return nil
				})
				results <- outcome{replayed, err}
			}(i)
		}
		close(start)
		replays := 0
		for i := 0; i < 2; i++ {
			r := <-results
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.replayed {
				replays++
			}
		}
		if replays != 1 || count(t, db, "payments") != 1 || count(t, db, "ledger_entries") != 1 || count(t, db, "audit_events") != 1 || get(t, store, o.ID()).Order.Paid() != 17500 {
			t.Fatal("concurrent replay duplicated financial posting")
		}
	})

	t.Run("parallel_distinct_keys_cannot_overpay", func(t *testing.T) {
		db, store := setup(t)
		o := newOrder(t, "order-1", customer, service)
		create(t, store, o)
		results := make(chan error, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func(i int) {
				<-start
				results <- store.WithinTransaction(ctx, func(tx repository.Tx) error {
					_, err := tx.RecordPayment(ctx, payment(fmt.Sprintf("p%d", i), o.ID(), fmt.Sprintf("key-%d", i), 10000), fmt.Sprintf("ledger-%d", i))
					return err
				})
			}(i)
		}
		close(start)
		success, denied := 0, 0
		for i := 0; i < 2; i++ {
			err := <-results
			if err == nil {
				success++
			} else {
				var e *domain.Error
				if !errors.As(err, &e) || e.Code != "overpayment" {
					t.Fatal(err)
				}
				denied++
			}
		}
		if success != 1 || denied != 1 || count(t, db, "payments") != 1 || count(t, db, "ledger_entries") != 1 || get(t, store, o.ID()).Order.Paid() != 10000 {
			t.Fatal("lost update or overpayment")
		}
	})

	t.Run("context_cancels_blocked_row_lock_and_releases_transaction", func(t *testing.T) {
		_, store := setup(t)
		o := newOrder(t, "order-1", customer, service)
		create(t, store, o)
		locked, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			finished <- store.WithinTransaction(ctx, func(tx repository.Tx) error {
				if _, err := tx.LockOrder(ctx, o.ID()); err != nil {
					return err
				}
				close(locked)
				<-release
				return nil
			})
		}()
		select {
		case <-locked:
		case err := <-finished:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		short, done := context.WithTimeout(ctx, 100*time.Millisecond)
		err := store.WithinTransaction(short, func(tx repository.Tx) error { _, err := tx.LockOrder(short, o.ID()); return err })
		done()
		close(release)
		if holderErr := <-finished; holderErr != nil {
			t.Fatal(holderErr)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline lost: %v", err)
		}
		if get(t, store, o.ID()).Version != 1 {
			t.Fatal("canceled transaction mutated order")
		}
	})

	t.Run("corrupt_persisted_snapshot_is_not_exposed", func(t *testing.T) {
		db, store := setup(t)
		o := newOrder(t, "order-1", customer, service)
		create(t, store, o)
		if _, err := db.ExecContext(ctx, `UPDATE public.orders SET subtotal=subtotal+1,total=total+1`); err != nil {
			t.Fatal(err)
		}
		err := store.View(ctx, func(r repository.Readers) error { _, err := r.GetOrder(ctx, o.ID()); return err })
		requireKind(t, err, repository.InvalidData)
	})

	t.Run("submicrosecond_events_are_canonicalized_consistently", func(t *testing.T) {
		_, store := setup(t)
		o, err := domain.NewOrder(domain.NewOrderInput{ID: "micro-order", CreatedAt: testAt.Add(900 * time.Nanosecond), Items: []domain.ItemRequest{{ServiceID: service.ID, Quantity: domain.Quantity{Grams: 2500}}}}, customer, []domain.Service{service}, actor)
		if err != nil {
			t.Fatal(err)
		}
		create(t, store, o)
		p := payment("micro-payment", o.ID(), "micro-key", 1000)
		p.ReceivedAt = testAt.Add(950 * time.Nanosecond)
		if err := store.WithinTransaction(ctx, func(tx repository.Tx) error { _, err := tx.RecordPayment(ctx, p, "micro-ledger"); return err }); err != nil {
			t.Fatal(err)
		}
		r := get(t, store, o.ID())
		if !r.Order.CreatedAt().Equal(testAt) || !r.Order.UpdatedAt().Equal(testAt) || r.Order.Paid() != 1000 {
			t.Fatal("microsecond chronology not preserved")
		}
	})

	t.Run("scan_iterator_and_callback_lifetime_fail_safely", func(t *testing.T) {
		_, store := setup(t)
		for _, query := range []string{`SELECT 'private-invalid-int'`, `SELECT CASE WHEN n=2 THEN 10/(n-2) ELSE 1 END FROM generate_series(1,2) AS n`} {
			err := store.View(ctx, func(r repository.Readers) error {
				s := r.(*session)
				_, err := queryAll(ctx, s.tx, "test read", query, func(rows *sql.Rows) (int, error) {
					var n int
					if err := rows.Scan(&n); err != nil {
						return 0, dbError(ctx, "test scan", err)
					}
					return n, nil
				})
				return err
			})
			requireKind(t, err, repository.Unavailable)
			if strings.Contains(err.Error(), "private-invalid-int") || strings.Contains(err.Error(), "division") {
				t.Fatal("raw scan/driver error exposed")
			}
		}
		var escaped repository.Readers
		if err := store.View(ctx, func(r repository.Readers) error { escaped = r; return nil }); err != nil {
			t.Fatal(err)
		}
		_, err := escaped.GetCustomer(ctx, customer.ID)
		requireKind(t, err, repository.Unavailable)
	})
}
