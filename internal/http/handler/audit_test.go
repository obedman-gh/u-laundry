package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type auditStub struct {
	list func(context.Context, string, repository.AuditFilter) (service.AuditHistory, error)
}

func (s auditStub) List(c context.Context, a string, f repository.AuditFilter) (service.AuditHistory, error) {
	return s.list(c, a, f)
}
func TestAuditHTTPBoundary(t *testing.T) {
	calls := 0
	h := New(Dependencies{Audit: auditStub{list: func(_ context.Context, a string, f repository.AuditFilter) (service.AuditHistory, error) {
		calls++
		if a != "actor-1" {
			t.Fatal(a)
		}
		return service.AuditHistory{Filter: f}, nil
	}}})
	for _, q := range []string{"from=bad", "from=2026-11-25T00:00:00Z", "action=password", "source=unknown", "actor_id=other/actor", "entity=unknown", "request_id=a&request_id=b", "limit=101", "offset=-1", "token=secret"} {
		w := protected(t, "GET", "/api/audit/events?"+q, "", h.AuditEvents)
		if w.Code != 400 || calls != 0 {
			t.Fatal(q, w.Code, calls)
		}
	}
	w := protected(t, "GET", "/api/audit/events", "", h.AuditEvents)
	var body dto.AuditHistoryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body.Items == nil || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	h = New(Dependencies{Audit: auditStub{list: func(_ context.Context, _ string, f repository.AuditFilter) (service.AuditHistory, error) {
		if f.Action != "payment_recorded" || f.RequestID != "server-1" || f.Page.Offset != 20 || f.Page.Limit != 20 || f.From.Hour() != 16 || f.To.Sub(f.From) != 24*time.Hour {
			t.Fatal("filter lost", f)
		}
		return service.AuditHistory{}, &service.AccessDenied{}
	}}})
	w = protected(t, "GET", "/api/audit/events?from=2026-11-24T00:00:00%2B08:00&to=2026-11-25T00:00:00%2B08:00&action=payment_recorded&request_id=server-1&offset=20", "", h.AuditEvents)
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}
