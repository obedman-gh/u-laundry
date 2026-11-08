package postgres

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"ulaundry/internal/repository"
	"ulaundry/internal/security"
)

var _ security.Store = (*Store)(nil)

func authEvent(ctx context.Context, tx *sql.Tx, e security.Event) error {
	if e.At.IsZero() || len(e.RequestID) == 0 || len(e.RequestID) > 100 {
		return security.ErrUnavailable
	}
	switch e.Kind {
	case "login_success", "login_failure", "login_limited", "logout", "access_denied":
	default:
		return security.ErrUnavailable
	}
	var actor any
	if e.ActorID != "" {
		actor = e.ActorID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO public.auth_events(kind,actor_id,request_id,occurred_at,permission) VALUES($1,$2,$3,$4,$5)`, e.Kind, actor, e.RequestID, databaseTime(e.At), e.Permission)
	return dbError(ctx, "append authentication event", err)
}
func (s *Store) Create(ctx context.Context, r security.Record, e security.Event) error {
	return s.WithinTransaction(ctx, func(t repository.Tx) error {
		tx := t.(*session).tx
		var hash, username, role string
		var active bool
		err := tx.QueryRowContext(ctx, `SELECT password_hash,username,role,active FROM public.staff WHERE id=$1 FOR SHARE`, r.UserID).Scan(&hash, &username, &role, &active)
		if errors.Is(err, sql.ErrNoRows) {
			return security.ErrUnauthorized
		}
		if err != nil {
			return dbError(ctx, "read session identity", err)
		}
		fingerprint := sha256.Sum256([]byte(hash))
		if !active || username != r.Username || role != r.Role || subtle.ConstantTimeCompare(fingerprint[:], r.Fingerprint[:]) != 1 {
			return security.ErrUnauthorized
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO public.staff_sessions(token_hash,staff_id,username,role,credential_fingerprint,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.Hash[:], r.UserID, r.Username, r.Role, r.Fingerprint[:], r.IssuedAt, r.ExpiresAt)
		if err != nil {
			return dbError(ctx, "create staff session", err)
		}
		return authEvent(ctx, tx, e)
	})
}
func (s *Store) Check(ctx context.Context, r security.Record) error {
	var fingerprint []byte
	var hash, username, role string
	var active bool
	err := s.db.QueryRowContext(ctx, `SELECT s.credential_fingerprint,u.password_hash,u.username,u.role,u.active FROM public.staff_sessions s JOIN public.staff u ON u.id=s.staff_id WHERE s.token_hash=$1 AND s.staff_id=$2 AND s.username=$3 AND s.role=$4 AND s.issued_at=$5 AND s.expires_at=$6 AND s.revoked_at IS NULL`, r.Hash[:], r.UserID, r.Username, r.Role, r.IssuedAt, r.ExpiresAt).Scan(&fingerprint, &hash, &username, &role, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return security.ErrUnauthorized
	}
	if err != nil {
		return dbError(ctx, "check staff session", err)
	}
	current := sha256.Sum256([]byte(hash))
	if !active || username != r.Username || role != r.Role || len(fingerprint) != 32 || subtle.ConstantTimeCompare(current[:], fingerprint) != 1 {
		return security.ErrUnauthorized
	}
	return nil
}
func (s *Store) Revoke(ctx context.Context, r security.Record, e security.Event) error {
	return s.WithinTransaction(ctx, func(t repository.Tx) error {
		tx := t.(*session).tx
		result, err := tx.ExecContext(ctx, `UPDATE public.staff_sessions SET revoked_at=$1 WHERE token_hash=$2 AND staff_id=$3 AND revoked_at IS NULL`, databaseTime(e.At), r.Hash[:], r.UserID)
		if err != nil {
			return dbError(ctx, "revoke staff session", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return dbError(ctx, "revoke session count", err)
		}
		if n == 0 {
			return nil
		}
		return authEvent(ctx, tx, e)
	})
}
func (s *Store) Audit(ctx context.Context, e security.Event) error {
	return s.WithinTransaction(ctx, func(t repository.Tx) error { return authEvent(ctx, t.(*session).tx, e) })
}
