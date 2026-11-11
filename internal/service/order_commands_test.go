package service

import (
	"context"
	"testing"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (t *fakeTx) LockOrderCommandKey(_ context.Context, _, _ string) error {
	return t.call("lock_command")
}
func (t *fakeTx) FindOrderCommand(_ context.Context, actor, key string) (repository.OrderCommandReceipt, error) {
	if err := t.call("find_command"); err != nil {
		return repository.OrderCommandReceipt{}, err
	}
	r, ok := t.state.commands[actor+"/"+key]
	if !ok {
		return r, missing()
	}
	return r, nil
}
func (t *fakeTx) SaveOrderCommand(_ context.Context, r repository.OrderCommandReceipt) error {
	if err := t.call("save_command"); err != nil {
		return err
	}
	if t.state.commands == nil {
		t.state.commands = map[string]repository.OrderCommandReceipt{}
	}
	t.state.commands[r.ActorID+"/"+r.Key] = r
	return nil
}
func (t *fakeTx) AppendOrderStatusEvent(_ context.Context, e domain.OrderStatusEvent) error {
	if err := t.call("status_event"); err != nil {
		return err
	}
	t.state.statusEvents = append(t.state.statusEvents, e)
	return nil
}
func (t *fakeTx) OrderHistory(_ context.Context, id string, _ repository.Page) ([]domain.OrderStatusEvent, error) {
	var result []domain.OrderStatusEvent
	for _, e := range t.state.statusEvents {
		if e.OrderID == id {
			result = append(result, e)
		}
	}
	return result, nil
}
func TestOrderCommandReplayAndRollback(t *testing.T) {
	u, rt := fixture(t, domain.Owner)
	ctx := context.Background()
	orders := NewOrders(u, rt)
	created, err := orders.Create(ctx, "actor-1", orderInput())
	if err != nil {
		t.Fatal(err)
	}
	r, err := orders.ChangeStatus(ctx, "actor-1", created.Order.ID(), domain.Processing, 1, "process-key")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Order.ProcessingAt().Equal(fixed) {
		t.Fatal("missing processing milestone")
	}
	replay, err := orders.Create(ctx, "actor-1", orderInput())
	if err != nil || !replay.Replayed || replay.Version != 2 || replay.TrackingToken != "" || replay.Order.ID() != created.Order.ID() {
		t.Fatal("create replay", err)
	}
	before := len(u.state.audits)
	r, err = orders.ChangeStatus(ctx, "actor-1", created.Order.ID(), domain.Processing, 1, "process-key")
	if err != nil || !r.Replayed || r.AppliedVersion != 2 || len(u.state.audits) != before {
		t.Fatal("status replay", err)
	}
	_, err = orders.ChangeStatus(ctx, "actor-1", created.Order.ID(), domain.Ready, 1, "process-key")
	requireDomain(t, err, domain.Conflict, "idempotency_conflict")
	for _, failAt := range []string{"status_event", "save_command", "audit"} {
		u.failAt = failAt
		_, err = orders.ChangeStatus(ctx, "actor-1", created.Order.ID(), domain.Ready, 2, "ready-key")
		if err == nil || u.state.orders[created.Order.ID()].Version != 2 {
			t.Fatal("failed command leaked", failAt, err)
		}
		if _, ok := u.state.commands["actor-1/ready-key"]; ok {
			t.Fatal("receipt survived rollback")
		}
	}
}
