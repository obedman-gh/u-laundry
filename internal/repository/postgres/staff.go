package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/security"
)

func (s *session) LockStaffManagement(ctx context.Context) error {
	_, err := s.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7643296101001)`)
	return dbError(ctx, "lock staff administration", err)
}
func (s *session) CountStaff(ctx context.Context, owners bool) (int, error) {
	var n int
	err := s.tx.QueryRowContext(ctx, `SELECT count(*) FROM public.staff WHERE NOT $1 OR (active AND role='owner')`, owners).Scan(&n)
	return n, dbError(ctx, "count staff", err)
}
func (s *session) LockStaff(ctx context.Context, id string) (repository.StaffRecord, error) {
	var v repository.StaffRecord
	err := s.tx.QueryRowContext(ctx, `SELECT `+staffColumns+`,version FROM public.staff WHERE id=$1 FOR UPDATE`, id).Scan(&v.Staff.ID, &v.Staff.Username, &v.Staff.Name, &v.Staff.Role, &v.Staff.Active, &v.Staff.CreatedAt, &v.Staff.UpdatedAt, &v.Version)
	if err != nil {
		return v, dbError(ctx, "lock staff", err)
	}
	if v.Staff.Validate() != nil || v.Version < 1 {
		return repository.StaffRecord{}, corrupt("lock staff")
	}
	return v, nil
}
func (s *session) ListStaff(ctx context.Context, page repository.Page) ([]repository.StaffRecord, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "list staff", `SELECT `+staffColumns+`,version FROM public.staff ORDER BY username,id LIMIT $1 OFFSET $2`, func(row *sql.Rows) (repository.StaffRecord, error) {
		var v repository.StaffRecord
		err := row.Scan(&v.Staff.ID, &v.Staff.Username, &v.Staff.Name, &v.Staff.Role, &v.Staff.Active, &v.Staff.CreatedAt, &v.Staff.UpdatedAt, &v.Version)
		if err != nil {
			return v, dbError(ctx, "scan staff", err)
		}
		if v.Staff.Validate() != nil || v.Version < 1 {
			return v, corrupt("list staff")
		}
		return v, nil
	}, p.Limit, p.Offset)
}
func (s *session) UpdateStaff(ctx context.Context, v domain.Staff, version int64, hash string) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if hash != "" {
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return corrupt("invalid replacement hash")
		}
	}
	result, err := s.tx.ExecContext(ctx, `UPDATE public.staff SET role=$1,active=$2,updated_at=$3,password_hash=CASE WHEN $4='' THEN password_hash ELSE $4 END,version=version+1 WHERE id=$5 AND version=$6`, v.Role, v.Active, databaseTime(v.UpdatedAt), hash, v.ID, version)
	if err != nil {
		return dbError(ctx, "update staff", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return dbError(ctx, "update staff count", err)
	}
	if n != 1 {
		return stale("update staff")
	}
	return nil
}
func (s *Store) RecordAccessDenied(ctx context.Context, actor, permission string) error {
	id := security.CorrelationID(ctx)
	if id == "" {
		id = rand.Text()
	}
	return s.Audit(ctx, security.Event{Kind: "access_denied", ActorID: actor, Permission: permission, RequestID: id, At: time.Now().UTC()})
}
