package service

import (
	"context"
	"ulaundry/internal/repository"
)

// Operations reuses the existing owner-only configuration permission, including
// current active staff checks and the access-denial audit decorator.
type Operations struct{ uow repository.UnitOfWork }

func NewOperations(uow repository.UnitOfWork) *Operations { return &Operations{uow} }
func (s *Operations) Authorize(ctx context.Context, actor string) error {
	return s.uow.View(ctx, func(r repository.Readers) error { _, err := authorize(ctx, r, actor, ManageConfiguration); return err })
}
