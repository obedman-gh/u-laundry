package service

import (
	"context"
	"errors"
	"testing"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type auditTestWork struct {
	repository.UnitOfWork
	filter repository.AuditFilter
	err    error
	calls  int
}
type auditTestReader struct {
	repository.Readers
	work *auditTestWork
}

func (w *auditTestWork) View(ctx context.Context, fn func(repository.Readers) error) error {
	return w.UnitOfWork.View(ctx, func(r repository.Readers) error { return fn(auditTestReader{r, w}) })
}
func (r auditTestReader) ListAudit(_ context.Context, f repository.AuditFilter) (repository.AuditPage, error) {
	r.work.filter = f
	r.work.calls++
	return repository.AuditPage{Items: []repository.AuditRecord{}}, r.work.err
}
func TestAuditTrailAuthorizationFiltersAndFailure(t *testing.T) {
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
		u, rt := fixture(t, role)
		w := &auditTestWork{UnitOfWork: u}
		v, err := NewAuditTrail(w, rt).List(context.Background(), "actor-1", repository.AuditFilter{})
		if role != domain.Owner {
			requireDomain(t, err, domain.Forbidden, "")
			if w.calls != 0 {
				t.Fatal("unauthorized query")
			}
			continue
		}
		if err != nil || v.Page.Items == nil || w.filter.Page.Limit != 20 || w.filter.To.Sub(w.filter.From) != 30*24*time.Hour || w.filter.From.Hour() != 16 {
			t.Fatal("WITA/default pagination", v, err)
		}
		w.err = failure
		v, err = NewAuditTrail(w, rt).List(context.Background(), "actor-1", repository.AuditFilter{})
		if !errors.Is(err, failure) || v.Page.Items != nil {
			t.Fatal("query failure masked", v, err)
		}
	}
	for _, f := range []repository.AuditFilter{{Action: "unknown"}, {Source: "other"}, {ActorID: "bad/id"}, {Entity: "secret"}, {RequestID: "bad\nvalue"}, {From: fixed}, {From: fixed, To: fixed.Add(367 * 24 * time.Hour)}, {Page: repository.Page{Limit: 101}}} {
		u, rt := fixture(t, domain.Owner)
		w := &auditTestWork{UnitOfWork: u}
		_, err := NewAuditTrail(w, rt).List(context.Background(), "actor-1", f)
		requireDomain(t, err, domain.Validation, "")
		if w.calls != 0 {
			t.Fatal("bad filter delegated")
		}
	}
}
