package repository

import (
	"context"
	"time"
	"ulaundry/internal/domain"
)

type AuditFilter struct {
	From, To                           time.Time
	Action, ActorID, RequestID, Source string
	Entity                             domain.AuditEntity
	EntityID                           string
	Page                               Page
}
type AuditRecord struct {
	Source, ID, ActorID, Action, RequestID string
	Entity                                 domain.AuditEntity
	EntityID                               string
	OccurredAt                             time.Time
	Metadata                               domain.AuditMetadata
	Permission                             string // Controlled auth permission, never a request payload.
}
type AuditPage struct {
	Items   []AuditRecord
	HasMore bool
}

func AuditActionValid(action string) bool {
	if domain.AuditAction(action).Valid() {
		return true
	}
	switch action {
	case "login_success", "login_failure", "login_limited", "logout", "access_denied":
		return true
	default:
		return false
	}
}
func (f AuditFilter) Validate() error {
	bad := func() error {
		return &domain.Error{Kind: domain.Validation, Code: "invalid_audit_filter", Message: "Filter audit tidak valid."}
	}
	if f.From.IsZero() || f.To.IsZero() || !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour || f.From.Year() < 1 || f.To.Year() > 9999 {
		return bad()
	}
	return f.ValidateFilters()
}
func (f AuditFilter) ValidateFilters() error {
	bad := func() error {
		return &domain.Error{Kind: domain.Validation, Code: "invalid_audit_filter", Message: "Filter audit tidak valid."}
	}
	if f.Action != "" && !AuditActionValid(f.Action) {
		return bad()
	}
	if f.Source != "" && f.Source != "business" && f.Source != "security" {
		return bad()
	}
	if f.Entity != "" && !f.Entity.Valid() && f.Entity != "authentication" {
		return bad()
	}
	for _, id := range []string{f.ActorID, f.EntityID, f.RequestID} {
		if id != "" && !domain.ValidAuditRequestID(id) {
			return bad()
		}
	}
	_, err := f.Page.Normalize()
	return err
}

type AuditQueries interface {
	ListAudit(context.Context, AuditFilter) (AuditPage, error)
}
