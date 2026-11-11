package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

var fixed = time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
var failure = errors.New("synthetic dependency failure")

type fakeState struct {
	staff            map[string]domain.Staff
	customers        map[string]domain.Customer
	customerVersions map[string]int64
	services         map[string]repository.ServiceRecord
	orders           map[string]repository.OrderRecord
	payments         map[string]domain.Payment
	ledger           map[string]domain.LedgerEntry
	tracking         map[[32]byte]string
	audits           []domain.AuditEvent
	revisions        []repository.ServiceRevision
	commands         map[string]repository.OrderCommandReceipt
	statusEvents     []domain.OrderStatusEvent
	totals           repository.LedgerTotals
	period           ReportPeriod
	filter           repository.OrderFilter
	list             []repository.OrderSummary
}

func copyMap[K comparable, V any](source map[K]V) map[K]V {
	target := make(map[K]V, len(source))
	for k, v := range source {
		target[k] = v
	}
	return target
}
func (s *fakeState) clone() *fakeState {
	c := *s
	c.staff = copyMap(s.staff)
	c.customers = copyMap(s.customers)
	c.customerVersions = copyMap(s.customerVersions)
	c.services = copyMap(s.services)
	c.orders = copyMap(s.orders)
	c.payments = copyMap(s.payments)
	c.ledger = copyMap(s.ledger)
	c.tracking = copyMap(s.tracking)
	c.audits = append([]domain.AuditEvent(nil), s.audits...)
	c.revisions = append([]repository.ServiceRevision(nil), s.revisions...)
	c.commands = copyMap(s.commands)
	c.statusEvents = append([]domain.OrderStatusEvent(nil), s.statusEvents...)
	return &c
}

type fakeUow struct {
	state                     *fakeState
	failAt                    string
	commitErr, viewErr        error
	commits, rollbacks, views int
	calls                     []string
}
type fakeTx struct {
	repository.Tx
	state *fakeState
	uow   *fakeUow
}

func (u *fakeUow) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	staged := u.state.clone()
	tx := &fakeTx{state: staged, uow: u}
	if err := fn(tx); err != nil {
		u.rollbacks++
		return err
	}
	if u.commitErr != nil {
		u.rollbacks++
		return u.commitErr
	}
	u.state = staged
	u.commits++
	return nil
}
func (u *fakeUow) View(ctx context.Context, fn func(repository.Readers) error) error {
	u.views++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if u.viewErr != nil {
		return u.viewErr
	}
	return fn(&fakeTx{state: u.state, uow: u})
}
func (t *fakeTx) call(op string) error {
	t.uow.calls = append(t.uow.calls, op)
	if t.uow.failAt == op {
		return failure
	}
	return nil
}
func missing() error {
	return &repository.Error{Kind: repository.NotFound, Operation: "fixture lookup"}
}
func (t *fakeTx) GetStaff(_ context.Context, id string) (domain.Staff, error) {
	if err := t.call("staff"); err != nil {
		return domain.Staff{}, err
	}
	v, ok := t.state.staff[id]
	if !ok {
		return v, missing()
	}
	return v, nil
}
func (t *fakeTx) GetCustomer(_ context.Context, id string) (domain.Customer, error) {
	if err := t.call("customer"); err != nil {
		return domain.Customer{}, err
	}
	v, ok := t.state.customers[id]
	if !ok {
		return v, missing()
	}
	return v, nil
}
func (t *fakeTx) GetService(_ context.Context, id string) (domain.Service, error) {
	if err := t.call("service"); err != nil {
		return domain.Service{}, err
	}
	v, ok := t.state.services[id]
	if !ok {
		return v.Service, missing()
	}
	return v.Service, nil
}
func (t *fakeTx) AddCustomer(_ context.Context, v domain.Customer) error {
	if err := t.call("add_customer"); err != nil {
		return err
	}
	t.state.customers[v.ID] = v
	return nil
}
func (t *fakeTx) AddService(_ context.Context, v domain.Service) error {
	if err := t.call("add_service"); err != nil {
		return err
	}
	t.state.services[v.ID] = repository.ServiceRecord{Service: v, Version: 1}
	return nil
}
func (t *fakeTx) LockService(_ context.Context, id string) (repository.ServiceRecord, error) {
	if err := t.call("lock_service"); err != nil {
		return repository.ServiceRecord{}, err
	}
	v, ok := t.state.services[id]
	if !ok {
		return v, missing()
	}
	return v, nil
}
func (t *fakeTx) UpdateService(_ context.Context, v domain.Service, version int64) error {
	if err := t.call("update_service"); err != nil {
		return err
	}
	t.state.services[v.ID] = repository.ServiceRecord{Service: v, Version: version + 1}
	return nil
}
func (t *fakeTx) CreateOrder(_ context.Context, v domain.Order) error {
	if err := t.call("create_order"); err != nil {
		return err
	}
	t.state.orders[v.ID()] = repository.OrderRecord{Order: v, Version: 1}
	return nil
}
func (t *fakeTx) AddTracking(_ context.Context, id string, hash [32]byte, expires time.Time) error {
	if err := t.call("tracking"); err != nil {
		return err
	}
	t.state.tracking[hash] = id
	return nil
}
func (t *fakeTx) AppendAudit(_ context.Context, e domain.AuditEvent) error {
	if err := t.call("audit"); err != nil {
		return err
	}
	t.state.audits = append(t.state.audits, e)
	return nil
}
func (t *fakeTx) LockOrder(ctx context.Context, id string) (repository.OrderRecord, error) {
	if err := t.call("lock_order"); err != nil {
		return repository.OrderRecord{}, err
	}
	return t.GetOrder(ctx, id)
}
func (t *fakeTx) GetOrder(_ context.Context, id string) (repository.OrderRecord, error) {
	if err := t.call("get_order"); err != nil {
		return repository.OrderRecord{}, err
	}
	v, ok := t.state.orders[id]
	if !ok {
		return v, missing()
	}
	return v, nil
}
func (t *fakeTx) SaveOrderProgress(_ context.Context, v domain.Order, version int64) error {
	if err := t.call("save_progress"); err != nil {
		return err
	}
	t.state.orders[v.ID()] = repository.OrderRecord{Order: v, Version: version + 1}
	return nil
}
func (t *fakeTx) FindPaymentByKey(_ context.Context, id, key string) (domain.Payment, error) {
	if err := t.call("find_payment"); err != nil {
		return domain.Payment{}, err
	}
	v, ok := t.state.payments[id+"|"+key]
	if !ok {
		return v, missing()
	}
	return v, nil
}
func (t *fakeTx) RecordPayment(_ context.Context, p domain.Payment, ledgerID string) (repository.PaymentResult, error) {
	if err := t.call("record_payment"); err != nil {
		return repository.PaymentResult{}, err
	}
	r, ok := t.state.orders[p.OrderID]
	if !ok {
		return repository.PaymentResult{}, missing()
	}
	if err := r.Order.RecordPayment(p); err != nil {
		return repository.PaymentResult{}, err
	}
	ledger, err := domain.NewPaymentLedger(ledgerID, p)
	if err != nil {
		return repository.PaymentResult{}, err
	}
	r.Version++
	t.state.orders[p.OrderID] = r
	t.state.payments[p.OrderID+"|"+p.IdempotencyKey] = p
	t.state.ledger[ledgerID] = ledger
	return repository.PaymentResult{Payment: p}, nil
}
func (t *fakeTx) GetTracking(_ context.Context, hash [32]byte, at time.Time) (repository.TrackingRecord, error) {
	if err := t.call("get_tracking"); err != nil {
		return repository.TrackingRecord{}, err
	}
	id, ok := t.state.tracking[hash]
	if !ok {
		return repository.TrackingRecord{}, missing()
	}
	o := t.state.orders[id].Order
	return repository.TrackingRecord{Status: o.Status(), EstimatedReadyAt: o.EstimatedReadyAt()}, nil
}
func (t *fakeTx) LedgerTotals(_ context.Context, from, to time.Time) (repository.LedgerTotals, error) {
	if err := t.call("report"); err != nil {
		return repository.LedgerTotals{}, err
	}
	t.state.period = ReportPeriod{from, to}
	return t.state.totals, nil
}
func (t *fakeTx) ListOrders(_ context.Context, f repository.OrderFilter) ([]repository.OrderSummary, error) {
	if err := t.call("list"); err != nil {
		return nil, err
	}
	t.state.filter = f
	return t.state.list, nil
}
func (t *fakeTx) FindCredentials(_ context.Context, username string) (repository.Credentials, error) {
	if err := t.call("credentials"); err != nil {
		return repository.Credentials{}, err
	}
	for _, a := range t.state.staff {
		if a.Username == username {
			return repository.Credentials{Staff: a, PasswordHash: "hash"}, nil
		}
	}
	return repository.Credentials{}, missing()
}

func fixture(t *testing.T, role domain.Role) (*fakeUow, Runtime) {
	t.Helper()
	a := domain.Staff{ID: "actor-1", Username: "staff.test", Name: "Staf Contoh", Role: role, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	c, err := domain.NewCustomer("customer-1", "Private Customer", "081234567890", "private notes", fixed)
	if err != nil {
		t.Fatal(err)
	}
	s := domain.Service{ID: "wash", Name: "Cuci Setrika", Unit: domain.PerKilogram, UnitPrice: 7000, Turnaround: 24 * time.Hour, Active: true, CreatedAt: fixed, UpdatedAt: fixed}
	o, err := domain.NewOrder(domain.NewOrderInput{ID: "order-1", CreatedAt: fixed, Items: []domain.ItemRequest{{ServiceID: s.ID, Quantity: domain.Quantity{Grams: 2500}}}}, c, []domain.Service{s}, a)
	if err != nil {
		t.Fatal(err)
	}
	u := &fakeUow{state: &fakeState{staff: map[string]domain.Staff{a.ID: a}, customers: map[string]domain.Customer{c.ID: c}, services: map[string]repository.ServiceRecord{s.ID: {Service: s, Version: 1}}, orders: map[string]repository.OrderRecord{o.ID(): {Order: o, Version: 1}}, payments: map[string]domain.Payment{}, ledger: map[string]domain.LedgerEntry{}, tracking: map[[32]byte]string{}}}
	index := 0
	rt := Runtime{Clock: ClockFunc(func() time.Time { return fixed }), IDs: GeneratorFunc(func() (string, error) { index++; return fmt.Sprintf("id-%03d", index), nil }), Tokens: GeneratorFunc(func() (string, error) { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32)), nil })}
	return u, rt
}
func requireDomain(t *testing.T, err error, kind domain.ErrorKind, code string) {
	t.Helper()
	var e *domain.Error
	if !errors.As(err, &e) || e.Kind != kind || (code != "" && e.Code != code) {
		t.Fatalf("expected %s/%s got %v", kind, code, err)
	}
}
func orderInput() OrderInput {
	return OrderInput{IdempotencyKey: "create-test", CustomerID: "customer-1", Items: []domain.ItemRequest{{ServiceID: "wash", Quantity: domain.Quantity{Grams: 2500}}}, Discount: 500, DiscountReason: "Penyesuaian layanan"}
}

func TestCreateOrderAuthoritativeCalculationAndAudit(t *testing.T) {
	u, rt := fixture(t, domain.Owner)
	result, err := NewOrders(u, rt).Create(context.Background(), "actor-1", orderInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.Order.Total() != 17000 || result.Order.Subtotal() != 17500 || result.Order.ID() != "id-001" || result.Order.CreatedBy() != "actor-1" || !result.Order.CreatedAt().Equal(fixed) || !result.Order.EstimatedReadyAt().Equal(fixed.Add(24*time.Hour)) {
		t.Fatal("server calculation, catalog snapshot, actor or clock incorrect")
	}
	if len(u.state.audits) != 2 || u.state.audits[0].Action != domain.OrderCreated || u.state.audits[1].Action != domain.DiscountApproved || u.commits != 1 {
		t.Fatal("missing atomic audits")
	}
	hash, ok := tokenHash(result.TrackingToken)
	if !ok || u.state.tracking[hash] != result.Order.ID() {
		t.Fatal("tracking not stored atomically")
	}
	view, err := NewTracking(u, Runtime{Clock: ClockFunc(func() time.Time { return fixed })}).Get(context.Background(), result.TrackingToken)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"Private", "081234", "private notes", result.TrackingToken, "17000", result.Order.ID()} {
		if strings.Contains(string(raw), sensitive) {
			t.Fatal("tracking exposed private data")
		}
	}
}

func TestOrderFailurePropagationAndRollbackIntent(t *testing.T) {
	for _, op := range []string{"staff", "customer", "lock_service", "create_order", "tracking", "audit", "commit"} {
		t.Run(op, func(t *testing.T) {
			u, rt := fixture(t, domain.Owner)
			u.failAt = op
			if op == "commit" {
				u.commitErr = failure
			}
			result, err := NewOrders(u, rt).Create(context.Background(), "actor-1", orderInput())
			if !errors.Is(err, failure) || result.Order.ID() != "" || result.TrackingToken != "" || u.commits != 0 || u.rollbacks != 1 || len(u.state.orders) != 1 || len(u.state.tracking) != 0 || len(u.state.audits) != 0 {
				t.Fatalf("failure/rollback intent not preserved: %v", err)
			}
		})
	}
	for _, which := range []string{"id", "token", "bad-token", "clock"} {
		t.Run(which, func(t *testing.T) {
			u, rt := fixture(t, domain.Owner)
			switch which {
			case "id":
				rt.IDs = GeneratorFunc(func() (string, error) { return "", failure })
			case "token":
				rt.Tokens = GeneratorFunc(func() (string, error) { return "", failure })
			case "bad-token":
				rt.Tokens = GeneratorFunc(func() (string, error) { return "private-invalid-token", nil })
			case "clock":
				rt.Clock = ClockFunc(func() time.Time { return time.Time{} })
			}
			result, err := NewOrders(u, rt).Create(context.Background(), "actor-1", orderInput())
			if err == nil || result.Order.ID() != "" || u.rollbacks != 1 || len(u.state.audits) != 0 {
				t.Fatal("dependency failure committed")
			}
			if strings.Contains(err.Error(), "private-invalid-token") {
				t.Fatal("token leaked")
			}
		})
	}
}

func TestAuthorizationAndInvalidOrder(t *testing.T) {
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
		for _, action := range []Action{CreateCustomer, CreateOrder, ChangeStatus, RecordPayment, ReadOrders} {
			if !Allowed(role, action) {
				t.Fatalf("operational permission missing %s %s", role, action)
			}
		}
	}
	for _, action := range []Action{ConfigureService, CancelOrder, GenerateReport} {
		if Allowed(domain.Cashier, action) || Allowed("unknown", action) {
			t.Fatal("cashier/unknown gained privileged action")
		}
	}
	if Allowed(domain.Owner, "unknown") {
		t.Fatal("unknown action allowed")
	}
	for _, which := range []string{"inactive", "missing", "discount-cashier", "inactive-service", "bad-quantity"} {
		t.Run(which, func(t *testing.T) {
			u, rt := fixture(t, domain.Cashier)
			in := orderInput()
			in.Discount = 0
			in.DiscountReason = ""
			actor := "actor-1"
			switch which {
			case "inactive":
				a := u.state.staff[actor]
				a.Active = false
				u.state.staff[actor] = a
			case "missing":
				actor = "missing"
			case "discount-cashier":
				in = orderInput()
			case "inactive-service":
				s := u.state.services["wash"]
				s.Service.Active = false
				u.state.services["wash"] = s
			case "bad-quantity":
				in.Items[0].Quantity.Grams = -1
			}
			_, err := NewOrders(u, rt).Create(context.Background(), actor, in)
			if err == nil || u.commits != 0 || len(u.state.orders) != 1 {
				t.Fatal("invalid/forbidden command applied")
			}
		})
	}
}

func TestCustomerAndCatalogWorkflows(t *testing.T) {
	u, rt := fixture(t, domain.Cashier)
	c, err := NewCustomers(u, rt).Create(context.Background(), "actor-1", CustomerInput{Name: "  Éka  ", Contact: "081234567890"})
	if err != nil || c.Name != "Éka" || c.Contact != "+6281234567890" || u.state.audits[0].Action != domain.CustomerCreated {
		t.Fatal("customer create", err)
	}
	_, err = NewCatalog(u, rt).Configure(context.Background(), "actor-1", ServiceInput{Name: "Cuci", Unit: domain.PerKilogram, UnitPrice: 7000, Turnaround: time.Hour, Active: true})
	requireDomain(t, err, domain.Forbidden, "forbidden")
	u, rt = fixture(t, domain.Admin)
	configured, err := NewCatalog(u, rt).Configure(context.Background(), "actor-1", ServiceInput{ID: "wash", ExpectedVersion: 1, Name: "Cuci Baru", Unit: domain.PerKilogram, UnitPrice: 8000, Turnaround: time.Hour, Active: true})
	if err != nil || configured.Version != 2 || configured.Service.UnitPrice != 8000 || !configured.Service.CreatedAt.Equal(fixed) {
		t.Fatal("configure existing", err)
	}
	_, err = NewCatalog(u, rt).Configure(context.Background(), "actor-1", ServiceInput{ID: "wash", ExpectedVersion: 1, Name: "Cuci Lama", Unit: domain.PerKilogram, UnitPrice: 1, Turnaround: time.Hour})
	requireDomain(t, err, domain.Conflict, "stale_service")
	if u.state.services["wash"].Service.UnitPrice != 8000 {
		t.Fatal("stale write applied")
	}
	created, err := NewCatalog(u, rt).Configure(context.Background(), "actor-1", ServiceInput{Name: "Satuan", Unit: domain.PerPiece, UnitPrice: 10000, Turnaround: time.Hour, Active: true})
	if err != nil || created.Version != 1 {
		t.Fatal("configure new", err)
	}
	for _, op := range []string{"add_customer", "add_service", "update_service", "audit"} {
		t.Run(op, func(t *testing.T) {
			u, rt := fixture(t, domain.Owner)
			u.failAt = op
			switch op {
			case "add_customer":
				_, err = NewCustomers(u, rt).Create(context.Background(), "actor-1", CustomerInput{Name: "Test"})
			case "update_service":
				_, err = NewCatalog(u, rt).Configure(context.Background(), "actor-1", ServiceInput{ID: "wash", ExpectedVersion: 1, Name: "Updated", Unit: domain.PerKilogram, UnitPrice: 1, Turnaround: time.Hour})
			default:
				_, err = NewCatalog(u, rt).Configure(context.Background(), "actor-1", ServiceInput{Name: "New", Unit: domain.PerPiece, UnitPrice: 1, Turnaround: time.Hour})
			}
			if !errors.Is(err, failure) || u.commits != 0 || len(u.state.services) != 1 || len(u.state.customers) != 1 {
				t.Fatal("configuration rollback intent", err)
			}
		})
	}
}

func TestTransitionsCancellationAndStaleState(t *testing.T) {
	ctx := context.Background()
	u, rt := fixture(t, domain.Owner)
	s := NewOrders(u, rt)
	_, err := s.ChangeStatus(ctx, "actor-1", "order-1", domain.Ready, 1, "invalid-key")
	requireDomain(t, err, domain.Conflict, "invalid_transition")
	if u.state.orders["order-1"].Order.Status() != domain.Received || len(u.state.audits) != 0 {
		t.Fatal("forbidden transition saved")
	}
	r, err := s.ChangeStatus(ctx, "actor-1", "order-1", domain.Processing, 1, "process-key")
	if err != nil || r.Version != 2 {
		t.Fatal(err)
	}
	_, err = s.ChangeStatus(ctx, "actor-1", "order-1", domain.Ready, 1, "stale-key")
	requireDomain(t, err, domain.Conflict, "stale_order")
	_, err = s.ChangeStatus(ctx, "actor-1", "order-1", domain.Cancelled, 2, "invalid-cancel")
	requireDomain(t, err, domain.Validation, "")
	r, err = s.Cancel(ctx, "actor-1", "order-1", 2, "Permintaan pelanggan", "cancel-key")
	if err != nil || r.Order.Status() != domain.Cancelled || u.state.audits[len(u.state.audits)-1].Action != domain.OrderCancelled {
		t.Fatal("cancel", err)
	}
	u, rt = fixture(t, domain.Cashier)
	_, err = NewOrders(u, rt).Cancel(ctx, "actor-1", "order-1", 1, "Permintaan pelanggan", "forbidden-cancel")
	requireDomain(t, err, domain.Forbidden, "forbidden")
	for _, op := range []string{"lock_order", "save_progress", "audit"} {
		t.Run(op, func(t *testing.T) {
			u, rt := fixture(t, domain.Owner)
			u.failAt = op
			_, err := NewOrders(u, rt).ChangeStatus(ctx, "actor-1", "order-1", domain.Processing, 1, "process-key")
			if !errors.Is(err, failure) || u.state.orders["order-1"].Version != 1 || u.state.orders["order-1"].Order.Status() != domain.Received {
				t.Fatal("status failure committed", err)
			}
		})
	}
}

func TestPaymentAtomicIntentReplayAndCommitFailure(t *testing.T) {
	ctx := context.Background()
	u, rt := fixture(t, domain.Cashier)
	s := NewPayments(u, rt)
	in := PaymentInput{OrderID: "order-1", IdempotencyKey: "key-1", Amount: 5000, Method: domain.Cash}
	result, err := s.Record(ctx, "actor-1", in)
	if err != nil || result.Replayed || len(u.state.ledger) != 1 || len(u.state.audits) != 1 || u.state.orders["order-1"].Order.Status() != domain.Received {
		t.Fatal("payment/audit", err)
	}
	brokenIDs := rt
	brokenIDs.IDs = GeneratorFunc(func() (string, error) { return "", failure })
	replay, err := NewPayments(u, brokenIDs).Record(ctx, "actor-1", in)
	if err != nil || !replay.Replayed || replay.Payment.ID != result.Payment.ID || len(u.state.audits) != 1 || len(u.state.ledger) != 1 {
		t.Fatal("replay posted again", err)
	}
	in.Amount++
	_, err = s.Record(ctx, "actor-1", in)
	requireDomain(t, err, domain.Conflict, "idempotency_conflict")
	for _, op := range []string{"find_payment", "record_payment", "audit", "commit"} {
		t.Run(op, func(t *testing.T) {
			u, rt := fixture(t, domain.Cashier)
			u.failAt = op
			if op == "commit" {
				u.commitErr = failure
			}
			result, err := NewPayments(u, rt).Record(ctx, "actor-1", PaymentInput{OrderID: "order-1", IdempotencyKey: "key-1", Amount: 5000, Method: domain.Cash})
			if !errors.Is(err, failure) || result.Payment.ID != "" || len(u.state.payments) != 0 || len(u.state.ledger) != 0 || len(u.state.audits) != 0 || u.state.orders["order-1"].Order.Paid() != 0 || u.rollbacks != 1 {
				t.Fatal("payment rollback intent or result leakage", err)
			}
		})
	}
}

func TestPaymentBusinessGuardsRunBeforePosting(t *testing.T) {
	for _, which := range []string{"overpayment", "cancelled", "missing-key"} {
		t.Run(which, func(t *testing.T) {
			u, rt := fixture(t, domain.Owner)
			in := PaymentInput{OrderID: "order-1", IdempotencyKey: "key-1", Amount: 5000, Method: domain.Cash}
			switch which {
			case "overpayment":
				in.Amount = 17501
			case "cancelled":
				record := u.state.orders["order-1"]
				if err := record.Order.Transition(domain.Cancelled, fixed, "Permintaan pelanggan"); err != nil {
					t.Fatal(err)
				}
				u.state.orders["order-1"] = record
			case "missing-key":
				in.IdempotencyKey = ""
			}
			_, err := NewPayments(u, rt).Record(context.Background(), "actor-1", in)
			if err == nil || u.commits != 0 || len(u.state.ledger) != 0 {
				t.Fatal("payment business guard missing")
			}
			for _, call := range u.calls {
				if call == "record_payment" {
					t.Fatal("invalid payment reached posting repository")
				}
			}
		})
	}
}

func TestTrackingReportAndReadAuthorization(t *testing.T) {
	ctx := context.Background()
	u, rt := fixture(t, domain.Owner)
	_, err := NewTracking(u, Runtime{Clock: ClockFunc(func() time.Time { return fixed })}).Get(ctx, "guessable-order-id")
	requireDomain(t, err, domain.NotFound, "tracking_not_found")
	if u.views != 0 {
		t.Fatal("malformed tracking queried storage")
	}
	token, _ := rt.Tokens.New()
	_, err = NewTracking(u, Runtime{Clock: ClockFunc(func() time.Time { return fixed })}).Get(ctx, token)
	requireDomain(t, err, domain.NotFound, "tracking_not_found")
	u.state.totals = repository.LedgerTotals{Inflow: 100000, Outflow: 45000}
	report, err := NewReports(u, rt).Generate(ctx, "actor-1", ReportPeriod{})
	if err != nil || report.Balance != 55000 || !report.From.Equal(time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC)) || report.To.Sub(report.From) != 24*time.Hour || u.state.audits[0].Action != domain.ReportGenerated {
		t.Fatal("report period/totals/audit", err)
	}
	_, err = NewReports(u, rt).Generate(ctx, "actor-1", ReportPeriod{From: fixed, To: fixed})
	requireDomain(t, err, domain.Validation, "")
	_, err = NewReports(u, rt).Generate(ctx, "actor-1", ReportPeriod{From: fixed, To: fixed.Add(time.Nanosecond)})
	requireDomain(t, err, domain.Validation, "")
	u.state.list = []repository.OrderSummary{{ID: "order-1", Total: 17500, Paid: 5000, Status: domain.Received}}
	items, err := NewOrders(u, rt).List(ctx, "actor-1", repository.OrderFilter{Status: domain.Received})
	if err != nil || len(items) != 1 || items[0].PaymentStatus != domain.PartiallyPaid || u.state.filter.Status != domain.Received {
		t.Fatal("list projection", err)
	}
	u.viewErr = failure
	_, err = NewOrders(u, rt).Get(ctx, "actor-1", "order-1")
	if !errors.Is(err, failure) {
		t.Fatal("read failure swallowed")
	}
	u, rt = fixture(t, domain.Cashier)
	_, err = NewReports(u, rt).Generate(ctx, "actor-1", ReportPeriod{})
	requireDomain(t, err, domain.Forbidden, "forbidden")
	for _, op := range []string{"report", "audit", "commit"} {
		t.Run(op, func(t *testing.T) {
			u, rt := fixture(t, domain.Admin)
			u.failAt = op
			if op == "commit" {
				u.commitErr = failure
			}
			r, err := NewReports(u, rt).Generate(ctx, "actor-1", ReportPeriod{})
			if !errors.Is(err, failure) || r.ID != "" || len(u.state.audits) != 0 {
				t.Fatal("report error leaked success", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = NewCustomers(u, rt).Create(ctx, "actor-1", CustomerInput{Name: "Test"})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}

type verifier struct{}

func (verifier) Compare(hash, password string) bool { return hash == "hash" && password == "correct" }

type issuer func(domain.Staff, time.Time) (string, error)

func (f issuer) Sign(a domain.Staff, at time.Time) (string, error) { return f(a, at) }
func TestAuthenticationGenericFailureAndAudit(t *testing.T) {
	ctx := context.Background()
	u, rt := fixture(t, domain.Cashier)
	sign := issuer(func(a domain.Staff, at time.Time) (string, error) {
		if a.ID != "actor-1" || !at.Equal(fixed) {
			t.Fatal("untrusted actor/clock")
		}
		return "signed", nil
	})
	s := NewAuth(u, rt, verifier{}, sign)
	for _, input := range [][2]string{{"missing", "correct"}, {"staff.test", "wrong"}, {"", ""}} {
		_, err := s.Login(ctx, input[0], input[1])
		if !errors.Is(err, ErrAuthentication) {
			t.Fatal("authentication failure differs", err)
		}
	}
	result, err := s.Login(ctx, "staff.test", "correct")
	if err != nil || result.Token != "signed" || len(u.state.audits) != 1 || u.state.audits[0].Action != domain.StaffLoggedIn {
		t.Fatal("login audit", err)
	}
	u, rt = fixture(t, domain.Cashier)
	u.commitErr = failure
	result, err = NewAuth(u, rt, verifier{}, sign).Login(ctx, "staff.test", "correct")
	if !errors.Is(err, failure) || result.Token != "" || len(u.state.audits) != 0 {
		t.Fatal("uncommitted token returned")
	}
}
