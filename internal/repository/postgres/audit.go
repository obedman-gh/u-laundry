package postgres

import (
	"context"
	"database/sql"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func (s *session) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	if err := e.Validate(); err != nil {
		return err
	}
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.audit_events (id,actor_id,action,entity,entity_id,occurred_at) VALUES ($1,$2,$3,$4,$5,$6)`, e.ID, e.ActorID, e.Action, e.Entity, e.EntityID, databaseTime(e.OccurredAt))
	return dbError(ctx, "append audit", err)
}
func (s *session) AuditForEntity(ctx context.Context, entity domain.AuditEntity, id string, page repository.Page) ([]domain.AuditEvent, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	if entity != domain.OrderEntity && entity != domain.PaymentEntity && entity != domain.CustomerEntity && entity != domain.ServiceEntity && entity != domain.StaffEntity && entity != domain.ReportEntity {
		return nil, &domain.Error{Kind: domain.Validation, Code: "invalid_entity", Field: "entity", Message: "Jenis objek audit tidak dikenal."}
	}
	return queryAll(ctx, s.tx, "read audit", `SELECT id,actor_id,action,entity,entity_id,occurred_at FROM public.audit_events WHERE entity=$1 AND entity_id=$2 ORDER BY occurred_at,id LIMIT $3 OFFSET $4`, func(row *sql.Rows) (domain.AuditEvent, error) {
		var e domain.AuditEvent
		if err := row.Scan(&e.ID, &e.ActorID, &e.Action, &e.Entity, &e.EntityID, &e.OccurredAt); err != nil {
			return e, dbError(ctx, "scan audit", err)
		}
		if err := e.Validate(); err != nil {
			return e, corrupt("read audit")
		}
		return e, nil
	}, entity, id, p.Limit, p.Offset)
}
