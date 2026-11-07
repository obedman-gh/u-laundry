package postgres

import (
	"context"
	"database/sql"
	"github.com/lib/pq"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

const orderColumns = `id,customer_id,customer_name,customer_contact,customer_notes,customer_created_at,customer_updated_at,created_by,item_count,subtotal,discount,discount_reason,discount_approved_by,total,notes,status,created_at,updated_at,estimated_ready_at,ready_at,collected_at,cancelled_at,cancellation_reason,version`

func optionalTime(at sql.NullTime) any {
	if at.Valid {
		return databaseTime(at.Time)
	}
	return nil
}
func nullTime(at domain.OrderState) (sql.NullTime, sql.NullTime, sql.NullTime) {
	return sql.NullTime{Time: at.ReadyAt, Valid: !at.ReadyAt.IsZero()}, sql.NullTime{Time: at.CollectedAt, Valid: !at.CollectedAt.IsZero()}, sql.NullTime{Time: at.CancelledAt, Valid: !at.CancelledAt.IsZero()}
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *session) GetOrder(ctx context.Context, id string) (repository.OrderRecord, error) {
	return s.loadOrder(ctx, id, false)
}
func (s *session) LockOrder(ctx context.Context, id string) (repository.OrderRecord, error) {
	return s.loadOrder(ctx, id, true)
}
func (s *session) loadOrder(ctx context.Context, id string, lock bool) (repository.OrderRecord, error) {
	query := `SELECT ` + orderColumns + ` FROM public.orders WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var state domain.OrderState
	var version int64
	var itemCount int
	var approved sql.NullString
	var ready, collected, cancelled sql.NullTime
	err := s.tx.QueryRowContext(ctx, query, id).Scan(&state.ID, &state.Customer.ID, &state.Customer.Name, &state.Customer.Contact, &state.Customer.Notes, &state.Customer.CreatedAt, &state.Customer.UpdatedAt, &state.CreatedBy, &itemCount,
		&state.Subtotal, &state.Discount.Amount, &state.Discount.Reason, &approved, &state.Total, &state.Notes, &state.Status, &state.CreatedAt, &state.UpdatedAt, &state.EstimatedReadyAt, &ready, &collected, &cancelled, &state.CancellationReason, &version)
	if err != nil {
		return repository.OrderRecord{}, dbError(ctx, "get order", err)
	}
	state.Discount.ApprovedBy = approved.String
	state.ReadyAt, state.CollectedAt, state.CancelledAt = ready.Time, collected.Time, cancelled.Time
	state.Items, err = queryAll(ctx, s.tx, "read order items", `SELECT line_no,service_id,service_name,unit,unit_price,pieces,grams,line_total FROM public.order_items WHERE order_id=$1 ORDER BY line_no`, func(row *sql.Rows) (domain.OrderItem, error) {
		var item domain.OrderItem
		var line int
		if err := row.Scan(&line, &item.ServiceID, &item.ServiceName, &item.Unit, &item.UnitPrice, &item.Quantity.Pieces, &item.Quantity.Grams, &item.LineTotal); err != nil {
			return item, dbError(ctx, "scan order item", err)
		}
		// A missing/duplicated line is detected below together with item_count.
		if line < 1 || line > itemCount {
			return item, corrupt("read order items")
		}
		return item, nil
	}, id)
	if err != nil {
		return repository.OrderRecord{}, err
	}
	if len(state.Items) != itemCount || version < 1 {
		return repository.OrderRecord{}, corrupt("read order")
	}
	state.Payments, err = queryAll(ctx, s.tx, "read order payments", `SELECT `+paymentColumns+` FROM public.payments WHERE order_id=$1 ORDER BY received_at,id`, func(row *sql.Rows) (domain.Payment, error) { return scanPayment(ctx, row) }, id)
	if err != nil {
		return repository.OrderRecord{}, err
	}
	o, err := domain.RestoreOrder(state)
	if err != nil {
		return repository.OrderRecord{}, corrupt("restore order")
	}
	return repository.OrderRecord{Order: o, Version: version}, nil
}

func (s *session) CreateOrder(ctx context.Context, o domain.Order) error {
	state := o.Snapshot()
	state.CreatedAt, state.UpdatedAt, state.EstimatedReadyAt = databaseTime(state.CreatedAt), databaseTime(state.UpdatedAt), databaseTime(state.EstimatedReadyAt)
	state.Customer.CreatedAt, state.Customer.UpdatedAt = databaseTime(state.Customer.CreatedAt), databaseTime(state.Customer.UpdatedAt)
	if _, err := domain.RestoreOrder(state); err != nil {
		return err
	}
	if state.Status != domain.Received || len(state.Payments) != 0 || !state.CreatedAt.Equal(state.UpdatedAt) {
		return &domain.Error{Kind: domain.Validation, Code: "order_not_new", Field: "order", Message: "Pembuatan hanya untuk pesanan baru tanpa pembayaran."}
	}
	ready, collected, cancelled := nullTime(state)
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.orders (`+orderColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		state.ID, state.Customer.ID, state.Customer.Name, state.Customer.Contact, state.Customer.Notes, state.Customer.CreatedAt, state.Customer.UpdatedAt, state.CreatedBy, len(state.Items),
		state.Subtotal, state.Discount.Amount, state.Discount.Reason, nullString(state.Discount.ApprovedBy), state.Total, state.Notes, state.Status, state.CreatedAt, state.UpdatedAt, state.EstimatedReadyAt, optionalTime(ready), optionalTime(collected), optionalTime(cancelled), state.CancellationReason, 1)
	if err != nil {
		return dbError(ctx, "create order", err)
	}
	for i, item := range state.Items {
		_, err = s.tx.ExecContext(ctx, `INSERT INTO public.order_items (order_id,line_no,service_id,service_name,unit,unit_price,pieces,grams,line_total) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, state.ID, i+1, item.ServiceID, item.ServiceName, item.Unit, item.UnitPrice, item.Quantity.Pieces, item.Quantity.Grams, item.LineTotal)
		if err != nil {
			return dbError(ctx, "create order item", err)
		}
	}
	return nil
}

func (s *session) ListOrders(ctx context.Context, filter repository.OrderFilter) ([]repository.OrderSummary, error) {
	p, err := filter.Page.Normalize()
	if err != nil {
		return nil, err
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, &domain.Error{Kind: domain.Validation, Code: "invalid_status", Field: "status", Message: "Filter status tidak dikenal."}
	}
	// Only fixed SQL fragments are concatenated; every filter value is a bind.
	return queryAll(ctx, s.tx, "list orders", `SELECT o.id,o.customer_id,o.customer_name,o.status,o.total,COALESCE((SELECT SUM(p.amount) FROM public.payments p WHERE p.order_id=o.id),0),o.created_at,o.estimated_ready_at,ARRAY(SELECT i.service_name FROM public.order_items i WHERE i.order_id=o.id ORDER BY i.line_no),o.version
		FROM public.orders o WHERE ($1::text='' OR o.status=$1) AND ($2::text='' OR o.customer_id=$2)
		ORDER BY o.created_at DESC,o.id DESC LIMIT $3 OFFSET $4`, func(row *sql.Rows) (repository.OrderSummary, error) {
		var item repository.OrderSummary
		if err := row.Scan(&item.ID, &item.CustomerID, &item.CustomerName, &item.Status, &item.Total, &item.Paid, &item.CreatedAt, &item.EstimatedReadyAt, (*pq.StringArray)(&item.ServiceNames), &item.Version); err != nil {
			return item, dbError(ctx, "scan order summary", err)
		}
		if !item.Status.Valid() || item.Total < 0 || item.Paid < 0 || item.Paid > item.Total {
			return item, corrupt("read order summary")
		}
		return item, nil
	}, string(filter.Status), filter.CustomerID, p.Limit, p.Offset)
}

func (s *session) SaveOrderProgress(ctx context.Context, o domain.Order, expectedVersion int64) error {
	stored, err := s.LockOrder(ctx, o.ID())
	if err != nil {
		return err
	}
	if expectedVersion != stored.Version {
		return stale("save order progress")
	}
	// Recheck against actual persisted payments/status, not a caller's stale copy.
	if err := stored.Order.Transition(o.Status(), databaseTime(o.UpdatedAt()), o.CancellationReason()); err != nil {
		return err
	}
	state := stored.Order.Snapshot()
	ready, collected, cancelled := nullTime(state)
	result, err := s.tx.ExecContext(ctx, `UPDATE public.orders SET status=$1,updated_at=$2,ready_at=$3,collected_at=$4,cancelled_at=$5,cancellation_reason=$6,version=version+1 WHERE id=$7 AND version=$8`,
		state.Status, state.UpdatedAt, optionalTime(ready), optionalTime(collected), optionalTime(cancelled), state.CancellationReason, state.ID, expectedVersion)
	if err != nil {
		return dbError(ctx, "save order progress", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "save order progress count", err)
	}
	if count != 1 {
		return stale("save order progress")
	}
	return nil
}
