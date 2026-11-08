package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type staffMemory struct {
	rows    map[string]repository.StaffRecord
	hashes  map[string]string
	events  []domain.AuditEvent
	denials []string
	fail    string
	commits int
}
type staffTx struct {
	repository.Tx
	rows   map[string]repository.StaffRecord
	hashes map[string]string
	events []domain.AuditEvent
	fail   string
}

func (m *staffMemory) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	t := &staffTx{rows: copyMap(m.rows), hashes: copyMap(m.hashes), events: append([]domain.AuditEvent(nil), m.events...), fail: m.fail}
	if err := fn(t); err != nil {
		return err
	}
	if m.fail == "commit" {
		return failure
	}
	m.rows, m.hashes, m.events = t.rows, t.hashes, t.events
	m.commits++
	return ctx.Err()
}
func (m *staffMemory) View(ctx context.Context, fn func(repository.Readers) error) error {
	return fn(&staffTx{rows: m.rows, hashes: m.hashes, events: m.events, fail: m.fail})
}
func (m *staffMemory) RecordAccessDenied(_ context.Context, actor, permission string) error {
	if m.fail == "denial" {
		return failure
	}
	m.denials = append(m.denials, actor+":"+permission)
	return nil
}
func (t *staffTx) LockStaffManagement(context.Context) error {
	if t.fail == "lock" {
		return failure
	}
	return nil
}
func (t *staffTx) GetStaff(_ context.Context, id string) (domain.Staff, error) {
	v, ok := t.rows[id]
	if !ok {
		return domain.Staff{}, repository.ErrNotFound
	}
	return v.Staff, nil
}
func (t *staffTx) LockStaff(_ context.Context, id string) (repository.StaffRecord, error) {
	v, ok := t.rows[id]
	if !ok {
		return v, repository.ErrNotFound
	}
	return v, nil
}
func (t *staffTx) CountStaff(_ context.Context, owners bool) (int, error) {
	n := 0
	for _, v := range t.rows {
		if !owners || v.Staff.Active && v.Staff.Role == domain.Owner {
			n++
		}
	}
	return n, nil
}
func (t *staffTx) ListStaff(_ context.Context, p repository.Page) ([]repository.StaffRecord, error) {
	if _, err := p.Normalize(); err != nil {
		return nil, err
	}
	out := []repository.StaffRecord{}
	for _, v := range t.rows {
		out = append(out, v)
	}
	return out, nil
}
func (t *staffTx) AddStaff(_ context.Context, v domain.Staff, hash string) error {
	for _, old := range t.rows {
		if old.Staff.Username == v.Username {
			return repository.ErrConflict
		}
	}
	t.rows[v.ID] = repository.StaffRecord{Staff: v, Version: 1}
	t.hashes[v.ID] = hash
	return nil
}
func (t *staffTx) UpdateStaff(_ context.Context, v domain.Staff, version int64, hash string) error {
	if t.fail == "update" {
		return failure
	}
	old := t.rows[v.ID]
	if old.Version != version {
		return repository.ErrConflict
	}
	t.rows[v.ID] = repository.StaffRecord{Staff: v, Version: version + 1}
	if hash != "" {
		t.hashes[v.ID] = hash
	}
	return nil
}
func (t *staffTx) AppendAudit(_ context.Context, e domain.AuditEvent) error {
	if t.fail == "audit" {
		return failure
	}
	if err := e.Validate(); err != nil {
		return err
	}
	t.events = append(t.events, e)
	return nil
}
func (t *staffTx) AuditForEntity(_ context.Context, _ domain.AuditEntity, _ string, _ repository.Page) ([]domain.AuditEvent, error) {
	return t.events, nil
}

type testHasher struct {
	fail  bool
	calls int
}

func (h *testHasher) Hash(string) (string, error) {
	h.calls++
	if h.fail {
		return "", failure
	}
	return "synthetic-hash", nil
}
func staffFixture(role domain.Role) (*staffMemory, *StaffManagement, *testHasher) {
	m := &staffMemory{rows: map[string]repository.StaffRecord{}, hashes: map[string]string{}}
	if role != "" {
		m.rows["actor"] = repository.StaffRecord{Staff: domain.Staff{ID: "actor", Username: "actor.test", Name: "Actor", Role: role, Active: true, CreatedAt: fixed, UpdatedAt: fixed}, Version: 1}
	}
	index := 0
	rt := Runtime{Clock: ClockFunc(func() time.Time { return fixed }), IDs: GeneratorFunc(func() (string, error) { index++; return fmt.Sprintf("staff-%d", index), nil })}
	h := &testHasher{}
	return m, NewStaffManagement(WithAccessAudit(m, m), rt, h), h
}
func TestPermissionMatrix(t *testing.T) {
	actions := []Action{ManageStaff, ManageConfiguration, ReadAudit, ApproveDiscount, ConfigureService, CancelOrder, GenerateReport, CreateCustomer, CreateOrder, ChangeStatus, RecordPayment, ReadOrders}
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier, "unknown"} {
		for _, action := range actions {
			want := role == domain.Owner || role == domain.Admin && action != ManageStaff && action != ManageConfiguration || role == domain.Cashier && (action == CreateCustomer || action == CreateOrder || action == ChangeStatus || action == RecordPayment || action == ReadOrders)
			if Allowed(role, action) != want {
				t.Fatalf("permission mismatch %s/%s", role, action)
			}
		}
		if Allowed(role, "unknown") {
			t.Fatal("unknown permission granted")
		}
	}
}
func TestStaffOwnerWorkflowsAndSafety(t *testing.T) {
	m, s, h := staffFixture(domain.Owner)
	ctx := context.Background()
	in := StaffInput{Username: "cashier.test", Name: "Synthetic Staff", Role: domain.Cashier, Password: "Frasa sandi sintetis"}
	created, err := s.Create(ctx, "actor", in)
	if err != nil || created.Version != 1 || len(m.events) != 1 || m.events[0].Action != domain.StaffCreated {
		t.Fatal("create failed", err)
	}
	for _, change := range []struct {
		op     string
		in     StaffChange
		action domain.AuditAction
	}{{"role", StaffChange{Version: 1, Role: domain.Admin}, domain.StaffRoleChanged}, {"password", StaffChange{Version: 2, Password: "Frasa pengganti sintetis"}, domain.StaffPasswordReset}, {"active", StaffChange{Version: 3, Active: false}, domain.StaffDeactivated}} {
		v, err := s.Change(ctx, "actor", created.Staff.ID, change.op, change.in)
		if err != nil || v.Version != change.in.Version+1 || m.events[len(m.events)-1].Action != change.action {
			t.Fatal("change failed", change.op, err)
		}
	}
	if h.calls != 2 || m.rows[created.Staff.ID].Staff.Active {
		t.Fatal("password hashing/status invalid")
	}
	for _, change := range []struct {
		op string
		in StaffChange
	}{{"active", StaffChange{Version: 1, Active: false}}, {"role", StaffChange{Version: 1, Role: domain.Admin}}} {
		_, err := s.Change(ctx, "actor", "actor", change.op, change.in)
		requireDomain(t, err, domain.Conflict, "self_lockout")
	}
	_, err = s.Change(ctx, "actor", created.Staff.ID, "role", StaffChange{Version: 1, Role: domain.Owner})
	requireDomain(t, err, domain.Conflict, "stale_staff")
}
func TestStaffNegativeRolesAndDenialAudit(t *testing.T) {
	for _, role := range []domain.Role{domain.Admin, domain.Cashier} {
		t.Run(string(role), func(t *testing.T) {
			m, s, h := staffFixture(role)
			in := StaffInput{Username: "new.test", Name: "Name", Role: domain.Owner, Password: "Frasa sandi sintetis"}
			_, err := s.Create(context.Background(), "actor", in)
			requireDomain(t, err, domain.Forbidden, "forbidden")
			_, err = s.List(context.Background(), "actor", repository.Page{})
			requireDomain(t, err, domain.Forbidden, "forbidden")
			_, err = s.Change(context.Background(), "actor", "nonexistent", "role", StaffChange{Version: 1, Role: domain.Owner})
			requireDomain(t, err, domain.Forbidden, "forbidden")
			if h.calls != 0 || len(m.rows) != 1 || len(m.events) != 0 || len(m.denials) != 3 {
				t.Fatal("denied call mutated or lost audit")
			}
		})
	}
	m, s, _ := staffFixture(domain.Cashier)
	m.fail = "denial"
	_, err := s.List(context.Background(), "actor", repository.Page{})
	if !errors.Is(err, failure) {
		t.Fatal("denial recorder failure ignored")
	}
}
func TestStaffRollbackInvalidAndBootstrap(t *testing.T) {
	for _, phase := range []string{"audit", "commit", "lock"} {
		t.Run(phase, func(t *testing.T) {
			m, s, _ := staffFixture(domain.Owner)
			m.fail = phase
			_, err := s.Create(context.Background(), "actor", StaffInput{Username: "new.test", Name: "Name", Role: domain.Admin, Password: "Frasa sandi sintetis"})
			if err == nil || len(m.rows) != 1 || len(m.events) != 0 {
				t.Fatal("partial staff transaction committed")
			}
		})
	}
	for _, in := range []StaffInput{{Username: "new.test", Name: "Name", Role: "invalid", Password: "Frasa sandi sintetis"}, {Username: "new.test", Name: "Name", Role: domain.Cashier, Password: "short"}} {
		m, s, _ := staffFixture(domain.Owner)
		_, err := s.Create(context.Background(), "actor", in)
		requireDomain(t, err, domain.Validation, "")
		if len(m.rows) != 1 {
			t.Fatal("invalid staff saved")
		}
	}
	m, s, _ := staffFixture("")
	owner, err := s.Bootstrap(context.Background(), StaffInput{Username: "first.owner", Name: "First", Password: "Frasa sandi sintetis"})
	if err != nil || owner.Staff.Role != domain.Owner || m.events[0].ActorID != owner.Staff.ID || m.events[0].Action != domain.StaffBootstrapped {
		t.Fatal("bootstrap invalid", err)
	}
	_, err = s.Bootstrap(context.Background(), StaffInput{Username: "second.owner", Name: "Second", Password: "Frasa sandi sintetis"})
	requireDomain(t, err, domain.Conflict, "bootstrap_closed")
	if len(m.rows) != 1 {
		t.Fatal("bootstrap overwrote existing staff")
	}
}
