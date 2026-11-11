package domain

import "time"

func ValidateIdempotencyKey(key string) error { return identifier("idempotency_key", key) }

// OrderStatusEvent records an accepted command, never a speculative transition.
// Baselines of older orders are read from storage without inventing past actors.
type OrderStatusEvent struct {
	OrderID, ActorID string
	Version          int64
	From, To         OrderStatus
	OccurredAt       time.Time
	Source           string
}

func (e OrderStatusEvent) Validate() error {
	if err := identifier("order_id", e.OrderID); err != nil {
		return err
	}
	if e.Version < 1 || e.OccurredAt.IsZero() || !e.To.Valid() {
		return invalid("event", "Histori status tidak valid.")
	}
	if e.Source == "baseline" && e.ActorID == "" && e.From == "" {
		return nil
	}
	if e.Source != "staff" {
		return invalid("event.source", "Sumber histori tidak valid.")
	}
	if err := identifier("actor_id", e.ActorID); err != nil {
		return err
	}
	if e.From == "" && e.To == Received && e.Version == 1 {
		return nil
	}
	if e.Version < 2 {
		return invalid("event.version", "Versi transisi tidak valid.")
	}
	return CheckTransition(e.From, e.To)
}
