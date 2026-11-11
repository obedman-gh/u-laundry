package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (s *session) LockOrderCommandKey(ctx context.Context, actor, key string) error {
	if err := domain.ValidateIdempotencyKey(key); err != nil {
		return err
	}
	// A hash collision only serializes unrelated commands. Receipt uniqueness
	// and payload comparison always use the complete actor/key, never this hash.
	hash := sha256.Sum256([]byte("ulaundry/order-command/" + actor + "/" + key))
	_, err := s.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(binary.BigEndian.Uint64(hash[:8])))
	return dbError(ctx, "lock order command", err)
}
func (s *session) FindOrderCommand(ctx context.Context, actor, key string) (repository.OrderCommandReceipt, error) {
	r := repository.OrderCommandReceipt{ActorID: actor, Key: key}
	var hash []byte
	err := s.tx.QueryRowContext(ctx, `SELECT operation,payload_hash,order_id,applied_version FROM public.order_command_receipts WHERE actor_id=$1 AND idempotency_key=$2`, actor, key).Scan(&r.Operation, &hash, &r.OrderID, &r.Version)
	if err != nil {
		return r, dbError(ctx, "read order command", err)
	}
	if len(hash) != 32 || r.Version < 1 {
		return r, corrupt("read order command")
	}
	copy(r.PayloadHash[:], hash)
	return r, nil
}
func (s *session) SaveOrderCommand(ctx context.Context, r repository.OrderCommandReceipt) error {
	if err := domain.ValidateIdempotencyKey(r.Key); err != nil {
		return err
	}
	result, err := s.tx.ExecContext(ctx, `INSERT INTO public.order_command_receipts(actor_id,idempotency_key,operation,payload_hash,order_id,applied_version)
SELECT $1,$2,$3,$4,id,version FROM public.orders WHERE id=$5 AND version=$6`, r.ActorID, r.Key, r.Operation, r.PayloadHash[:], r.OrderID, r.Version)
	if err != nil {
		return dbError(ctx, "save order command", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "save order command count", err)
	}
	if n != 1 {
		return stale("save order command")
	}
	return nil
}
func (s *session) AppendOrderStatusEvent(ctx context.Context, e domain.OrderStatusEvent) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Source != "staff" {
		return corrupt("append order status event")
	}
	result, err := s.tx.ExecContext(ctx, `INSERT INTO public.order_status_events(order_id,version,from_status,to_status,actor_id,occurred_at,source)
SELECT id,version,$3,status,$4,updated_at,'staff' FROM public.orders WHERE id=$1 AND version=$2 AND status=$5 AND updated_at=$6`, e.OrderID, e.Version, nullString(string(e.From)), e.ActorID, e.To, databaseTime(e.OccurredAt))
	if err != nil {
		return dbError(ctx, "append order status event", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "append order status event count", err)
	}
	if n != 1 {
		return stale("append order status event")
	}
	return nil
}
func (s *session) OrderHistory(ctx context.Context, id string, page repository.Page) ([]domain.OrderStatusEvent, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "order history", `SELECT order_id,version,from_status,to_status,actor_id,occurred_at,source FROM public.order_status_events WHERE order_id=$1 ORDER BY version DESC LIMIT $2 OFFSET $3`, func(row *sql.Rows) (domain.OrderStatusEvent, error) {
		var e domain.OrderStatusEvent
		var from, actor sql.NullString
		if err := row.Scan(&e.OrderID, &e.Version, &from, &e.To, &actor, &e.OccurredAt, &e.Source); err != nil {
			return e, dbError(ctx, "scan order history", err)
		}
		e.From = domain.OrderStatus(from.String)
		e.ActorID = actor.String
		if e.Validate() != nil {
			return e, corrupt("read order history")
		}
		return e, nil
	}, id, p.Limit, p.Offset)
}
