package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/security"
)

func (s *session) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	if id := security.CorrelationID(ctx); id != "" {
		e.RequestID = id
	}
	if e.RequestID == "" {
		e.RequestID = rand.Text()
	}
	if err := e.Validate(); err != nil {
		return err
	}
	metadata, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}
	_, err = s.tx.ExecContext(ctx, `INSERT INTO public.audit_events (id,actor_id,action,entity,entity_id,occurred_at,request_id,metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, e.ID, e.ActorID, e.Action, e.Entity, e.EntityID, databaseTime(e.OccurredAt), e.RequestID, string(metadata))
	return dbError(ctx, "append audit", err)
}
func (s *session) AuditForEntity(ctx context.Context, entity domain.AuditEntity, id string, page repository.Page) ([]domain.AuditEvent, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	if !entity.Valid() {
		return nil, &domain.Error{Kind: domain.Validation, Code: "invalid_entity", Field: "entity", Message: "Jenis objek audit tidak dikenal."}
	}
	return queryAll(ctx, s.tx, "read audit", `SELECT id,actor_id,action,entity,entity_id,occurred_at,COALESCE(request_id,''),metadata FROM public.audit_events WHERE entity=$1 AND entity_id=$2 ORDER BY occurred_at,id LIMIT $3 OFFSET $4`, func(row *sql.Rows) (domain.AuditEvent, error) {
		var e domain.AuditEvent
		var metadata []byte
		if err := row.Scan(&e.ID, &e.ActorID, &e.Action, &e.Entity, &e.EntityID, &e.OccurredAt, &e.RequestID, &metadata); err != nil {
			return e, dbError(ctx, "scan audit", err)
		}
		if err := json.Unmarshal(metadata, &e.Metadata); err != nil {
			return e, corrupt("read audit metadata")
		}
		if err := e.Validate(); err != nil {
			return e, corrupt("read audit")
		}
		return e, nil
	}, entity, id, p.Limit, p.Offset)
}

func (s *session) ListAudit(ctx context.Context, f repository.AuditFilter) (repository.AuditPage, error) {
	if err := f.Validate(); err != nil {
		return repository.AuditPage{}, err
	}
	p, err := f.Page.Normalize()
	if err != nil {
		return repository.AuditPage{}, err
	}
	items, err := queryAll(ctx, s.tx, "list audit", `SELECT source,id,actor_id,action,entity,entity_id,occurred_at,request_id,metadata,permission FROM (
 SELECT 'business'::text AS source,id,actor_id,action,entity,entity_id,occurred_at,COALESCE(request_id,'') AS request_id,metadata,''::text AS permission
 FROM public.audit_events WHERE occurred_at >= $1 AND occurred_at < $2
 UNION ALL
 SELECT 'security',id::text,COALESCE(actor_id,''),kind,
 CASE WHEN actor_id IS NULL THEN 'authentication' ELSE 'staff' END,COALESCE(actor_id,''),occurred_at,request_id,'{}'::jsonb,permission
 FROM public.auth_events WHERE occurred_at >= $1 AND occurred_at < $2
 ) events WHERE ($3::text='' OR action=$3) AND ($4::text='' OR actor_id=$4)
 AND ($5::text='' OR entity=$5) AND ($6::text='' OR entity_id=$6)
 AND ($7::text='' OR request_id=$7) AND ($8::text='' OR source=$8)
 ORDER BY occurred_at DESC,source DESC,id DESC LIMIT $9 OFFSET $10`, func(row *sql.Rows) (repository.AuditRecord, error) {
		var e repository.AuditRecord
		var metadata []byte
		if err := row.Scan(&e.Source, &e.ID, &e.ActorID, &e.Action, &e.Entity, &e.EntityID, &e.OccurredAt, &e.RequestID, &metadata, &e.Permission); err != nil {
			return e, dbError(ctx, "scan audit history", err)
		}
		if err := json.Unmarshal(metadata, &e.Metadata); err != nil {
			return e, corrupt("audit history metadata")
		}
		if !repository.AuditActionValid(e.Action) || e.Metadata.Validate() != nil {
			return e, corrupt("audit history")
		}
		return e, nil
	}, f.From, f.To, f.Action, f.ActorID, f.Entity, f.EntityID, f.RequestID, f.Source, p.Limit+1, p.Offset)
	if err != nil {
		return repository.AuditPage{}, err
	}
	v := repository.AuditPage{Items: items, HasMore: len(items) > p.Limit}
	if v.HasMore {
		v.Items = items[:p.Limit]
	}
	return v, nil
}
