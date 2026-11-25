package postgres

import (
	"context"
	"database/sql"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

// Payment receipts stay immutable after a refund. Subtract refunded receipts
// when calculating current debt; do not infer money from operational statuses.
const reportPaid = `WITH paid AS (
 SELECT p.order_id,SUM(p.amount)::bigint AS amount FROM public.payments p
 WHERE NOT EXISTS(SELECT 1 FROM public.refunds r WHERE r.payment_id=p.id)
 GROUP BY p.order_id
) `

func (s *session) DashboardMetrics(ctx context.Context, w repository.ReportWindow) (repository.DashboardMetrics, error) {
	var v repository.DashboardMetrics
	err := s.tx.QueryRowContext(ctx, reportPaid+`SELECT
 count(*) FILTER(WHERE o.created_at >= $1 AND o.created_at < $2),
 count(*) FILTER(WHERE o.status='received'), count(*) FILTER(WHERE o.status='processing'),
 count(*) FILTER(WHERE o.status='ready'),
 count(*) FILTER(WHERE o.status IN ('received','processing') AND o.estimated_ready_at < $3),
 count(*) FILTER(WHERE o.status <> 'cancelled' AND o.total > COALESCE(p.amount,0)),
 COALESCE(SUM(o.total-COALESCE(p.amount,0)) FILTER(WHERE o.status <> 'cancelled'),0)::bigint
 FROM public.orders o LEFT JOIN paid p ON p.order_id=o.id`, w.From, w.To, w.AsOf).Scan(
		&v.Orders.Created, &v.Orders.Received, &v.Orders.Processing, &v.Orders.Ready,
		&v.Orders.Overdue, &v.Orders.ReceivableOrders, &v.Orders.Receivable)
	if err != nil {
		return v, dbError(ctx, "dashboard orders", err)
	}
	if v.Orders.Receivable < 0 {
		return v, corrupt("dashboard receivable")
	}
	// Separate receipts, expenses and drawer transfers. Opening/closing balances
	// are never posted here and must never become revenue.
	err = s.tx.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(amount) FILTER(WHERE payment_id IS NOT NULL),0)::bigint,
 COALESCE(SUM(amount) FILTER(WHERE refund_id IS NOT NULL),0)::bigint,
 COALESCE(SUM(CASE WHEN payment_id IS NOT NULL THEN amount WHEN refund_id IS NOT NULL THEN -amount ELSE 0 END),0)::bigint,
 COALESCE(SUM(amount) FILTER(WHERE type='KREDIT' AND refund_id IS NULL AND NOT EXISTS(SELECT 1 FROM public.shift_cash_movements m WHERE m.ledger_id=l.id)),0)::bigint,
 COALESCE(SUM(amount) FILTER(WHERE type='DEBIT' AND EXISTS(SELECT 1 FROM public.shift_cash_movements m WHERE m.ledger_id=l.id)),0)::bigint,
 COALESCE(SUM(amount) FILTER(WHERE type='KREDIT' AND EXISTS(SELECT 1 FROM public.shift_cash_movements m WHERE m.ledger_id=l.id)),0)::bigint,
 COALESCE(SUM(amount) FILTER(WHERE payment_id IS NOT NULL AND shift_id IS NULL),0)::bigint,
 COALESCE(SUM(amount) FILTER(WHERE refund_id IS NOT NULL AND shift_id IS NULL),0)::bigint
 FROM public.ledger_entries l WHERE occurred_at >= $1 AND occurred_at < $2`, w.From, w.To).Scan(
		&v.Money.Payments, &v.Money.Refunds, &v.Money.Net, &v.Money.Expenses, &v.Money.CashIn, &v.Money.CashOut, &v.Money.UnassignedPayments, &v.Money.UnassignedRefunds)
	if err != nil {
		return v, dbError(ctx, "dashboard money", err)
	}
	v.Methods, err = queryAll(ctx, s.tx, "dashboard methods", `SELECT methods.method,
 COALESCE(SUM(l.amount) FILTER(WHERE l.payment_id IS NOT NULL),0)::bigint,
 COALESCE(SUM(l.amount) FILTER(WHERE l.refund_id IS NOT NULL),0)::bigint
 FROM (VALUES ('cash'),('bank_transfer'),('qris'),('unknown')) AS methods(method)
 LEFT JOIN public.ledger_entries l ON COALESCE(l.method,'unknown')=methods.method
 AND l.occurred_at >= $1 AND l.occurred_at < $2
 GROUP BY methods.method ORDER BY methods.method`, func(row *sql.Rows) (repository.MethodMetrics, error) {
		var m repository.MethodMetrics
		err := row.Scan(&m.Method, &m.Payments, &m.Refunds)
		return m, dbError(ctx, "scan dashboard method", err)
	}, w.From, w.To)
	if err != nil {
		return v, err
	}
	err = s.tx.QueryRowContext(ctx, `SELECT
 count(*) FILTER(WHERE opened_at >= $1 AND opened_at < $2),
 count(*) FILTER(WHERE closed_at >= $1 AND closed_at < $2),
 count(*) FILTER(WHERE closed_at IS NULL),
 COALESCE(SUM(system_cash) FILTER(WHERE closed_at >= $1 AND closed_at < $2),0)::bigint,
 COALESCE(SUM(physical_cash) FILTER(WHERE closed_at >= $1 AND closed_at < $2),0)::bigint,
 COALESCE(SUM(difference) FILTER(WHERE closed_at >= $1 AND closed_at < $2),0)::bigint
 FROM public.cash_shifts`, w.From, w.To).Scan(&v.Shifts.Opened, &v.Shifts.Closed, &v.Shifts.Active, &v.Shifts.System, &v.Shifts.Physical, &v.Shifts.Difference)
	if err != nil {
		return v, dbError(ctx, "dashboard shifts", err)
	}
	v.Popular, err = queryAll(ctx, s.tx, "dashboard popular services", `SELECT i.service_id,i.service_name,i.unit,
 count(DISTINCT i.order_id),SUM(i.pieces)::bigint,SUM(i.grams)::bigint,SUM(i.line_total)::bigint
 FROM public.order_items i JOIN public.orders o ON o.id=i.order_id
 WHERE o.created_at >= $1 AND o.created_at < $2 AND o.status <> 'cancelled'
 GROUP BY i.service_id,i.service_name,i.unit
 ORDER BY count(DISTINCT i.order_id) DESC,i.service_id,i.service_name,i.unit LIMIT 10`, func(row *sql.Rows) (repository.PopularService, error) {
		var p repository.PopularService
		err := row.Scan(&p.ID, &p.Name, &p.Unit, &p.Orders, &p.Pieces, &p.Grams, &p.Gross)
		return p, dbError(ctx, "scan popular service", err)
	}, w.From, w.To)
	return v, err
}

func (s *session) ReportOrders(ctx context.Context, f repository.ReportOrderFilter) (repository.ReportPage[repository.ReportOrder], error) {
	p, err := f.Page.Normalize()
	if err != nil {
		return repository.ReportPage[repository.ReportOrder]{}, err
	}
	if (f.Kind != "created" && f.Kind != "receivable" && f.Kind != "overdue") || (f.Status != "" && !f.Status.Valid()) {
		return repository.ReportPage[repository.ReportOrder]{}, &domain.Error{Kind: domain.Validation, Code: "invalid_report_filter", Message: "Filter laporan tidak valid."}
	}
	items, err := queryAll(ctx, s.tx, "report orders", reportPaid+`SELECT o.id,o.customer_name,o.status,o.total,COALESCE(p.amount,0),
 CASE WHEN o.status='cancelled' THEN 0 ELSE o.total-COALESCE(p.amount,0) END,o.created_at,o.estimated_ready_at
 FROM public.orders o LEFT JOIN paid p ON p.order_id=o.id
 WHERE ($1::text='' OR o.status=$1) AND
 (($2='created' AND o.created_at >= $3 AND o.created_at < $4)
 OR ($2='receivable' AND o.status <> 'cancelled' AND o.total > COALESCE(p.amount,0))
 OR ($2='overdue' AND o.status IN ('received','processing') AND o.estimated_ready_at < $5))
 ORDER BY o.created_at DESC,o.id DESC LIMIT $6 OFFSET $7`, func(row *sql.Rows) (repository.ReportOrder, error) {
		var v repository.ReportOrder
		if err := row.Scan(&v.ID, &v.CustomerName, &v.Status, &v.Total, &v.Paid, &v.Due, &v.CreatedAt, &v.EstimatedReadyAt); err != nil {
			return v, dbError(ctx, "scan report order", err)
		}
		if !v.Status.Valid() || v.Paid < 0 || v.Paid > v.Total || v.Due < 0 {
			return v, corrupt("report order")
		}
		return v, nil
	}, f.Status, f.Kind, f.Window.From, f.Window.To, f.Window.AsOf, p.Limit+1, p.Offset)
	return reportPage(items, p.Limit), err
}

func (s *session) ReportLedger(ctx context.Context, w repository.ReportWindow, page repository.Page) (repository.ReportPage[domain.LedgerEntry], error) {
	p, err := page.Normalize()
	if err != nil {
		return repository.ReportPage[domain.LedgerEntry]{}, err
	}
	items, err := queryAll(ctx, s.tx, "report ledger", `SELECT `+ledgerColumns+` FROM public.ledger_entries WHERE occurred_at >= $1 AND occurred_at < $2 ORDER BY occurred_at,id LIMIT $3 OFFSET $4`, func(row *sql.Rows) (domain.LedgerEntry, error) { return scanLedger(ctx, row) }, w.From, w.To, p.Limit+1, p.Offset)
	return reportPage(items, p.Limit), err
}

func reportPage[T any](items []T, limit int) repository.ReportPage[T] {
	v := repository.ReportPage[T]{Items: items, HasMore: len(items) > limit}
	if v.HasMore {
		v.Items = items[:limit]
	}
	return v
}
