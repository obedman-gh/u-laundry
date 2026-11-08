package service

import (
	"context"
	"errors"
	"ulaundry/internal/repository"
)

type AccessDenied struct {
	ActorID    string
	Permission Action
}

func (e *AccessDenied) Error() string { return "Akses tidak diizinkan." }
func (e *AccessDenied) Unwrap() error { return forbidden() }

type AccessRecorder interface {
	RecordAccessDenied(context.Context, string, string) error
}
type auditedWork struct {
	base     repository.UnitOfWork
	recorder AccessRecorder
}

// WithAccessAudit records denials only after the business transaction/view has
// finished, so the event survives rollback and works with a one-connection pool.
func WithAccessAudit(base repository.UnitOfWork, recorder AccessRecorder) repository.UnitOfWork {
	return &auditedWork{base, recorder}
}
func (u *auditedWork) record(ctx context.Context, err error) error {
	var denial *AccessDenied
	if errors.As(err, &denial) {
		if auditErr := u.recorder.RecordAccessDenied(ctx, denial.ActorID, string(denial.Permission)); auditErr != nil {
			return auditErr
		}
	}
	return err
}
func (u *auditedWork) View(ctx context.Context, fn func(repository.Readers) error) error {
	return u.record(ctx, u.base.View(ctx, fn))
}
func (u *auditedWork) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	return u.record(ctx, u.base.WithinTransaction(ctx, fn))
}
