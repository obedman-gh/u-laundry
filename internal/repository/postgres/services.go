package postgres

import (
	"context"
	"database/sql"
	"ulaundry/internal/repository"
)

func scanServiceRecord(ctx context.Context, row scanner) (repository.ServiceRecord, error) {
	var r repository.ServiceRecord
	v := &r.Service
	if err := row.Scan(&v.ID, &v.Name, &v.Unit, &v.UnitPrice, &v.Turnaround, &v.Active, &v.CreatedAt, &v.UpdatedAt, &v.DisplayOrder, &r.Version); err != nil {
		return r, dbError(ctx, "read service record", err)
	}
	if v.Validate() != nil || r.Version < 1 {
		return repository.ServiceRecord{}, corrupt("read service record")
	}
	return r, nil
}
func (s *session) GetServiceRecord(ctx context.Context, id string) (repository.ServiceRecord, error) {
	return scanServiceRecord(ctx, s.tx.QueryRowContext(ctx, `SELECT `+serviceColumns+`,version FROM public.services WHERE id=$1`, id))
}
func (s *session) ListServices(ctx context.Context, filter repository.ServiceFilter) ([]repository.ServiceRecord, error) {
	p, err := filter.Page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "list catalog", `SELECT `+serviceColumns+`,version FROM public.services WHERE ($1 OR active) ORDER BY display_order,name,id LIMIT $2 OFFSET $3`, func(row *sql.Rows) (repository.ServiceRecord, error) { return scanServiceRecord(ctx, row) }, filter.IncludeInactive, p.Limit, p.Offset)
}
func (s *session) AppendServiceRevision(ctx context.Context, r repository.ServiceRevision) error {
	if err := r.Service.Validate(); err != nil {
		return err
	}
	if r.Version < 1 || r.ActorID == "" || (r.Source != "" && r.Source != "staff") {
		return corrupt("append service revision")
	}
	// Copy the stored catalog row so history cannot record client-selected prices.
	result, err := s.tx.ExecContext(ctx, `INSERT INTO public.service_revisions(service_id,version,name,unit,unit_price,turnaround_ns,active,display_order,changed_by,changed_at,source)
	SELECT id,version,name,unit,unit_price,turnaround_ns,active,display_order,$3,updated_at,'staff' FROM public.services WHERE id=$1 AND version=$2`, r.Service.ID, r.Version, r.ActorID)
	if err != nil {
		return dbError(ctx, "append service revision", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "append service revision count", err)
	}
	if n != 1 {
		return stale("append service revision")
	}
	return nil
}
func (s *session) ServiceHistory(ctx context.Context, id string, page repository.Page) ([]repository.ServiceRevision, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "service history", `SELECT h.service_id,h.version,h.name,h.unit,h.unit_price,h.turnaround_ns,h.active,h.display_order,h.changed_by,h.changed_at,h.source,s.created_at FROM public.service_revisions h JOIN public.services s ON s.id=h.service_id WHERE h.service_id=$1 ORDER BY h.version DESC LIMIT $2 OFFSET $3`, func(row *sql.Rows) (repository.ServiceRevision, error) {
		var r repository.ServiceRevision
		v := &r.Service
		var actor sql.NullString
		var source string
		if err := row.Scan(&v.ID, &r.Version, &v.Name, &v.Unit, &v.UnitPrice, &v.Turnaround, &v.Active, &v.DisplayOrder, &actor, &v.UpdatedAt, &source, &v.CreatedAt); err != nil {
			return r, dbError(ctx, "scan service history", err)
		}
		r.ActorID = actor.String
		r.Source = source
		if v.Validate() != nil || r.Version < 1 {
			return repository.ServiceRevision{}, corrupt("service history")
		}
		return r, nil
	}, id, p.Limit, p.Offset)
}
