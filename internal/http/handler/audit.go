package handler

import (
	"net/http"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
)

func (h *Handler) AuditEvents(w http.ResponseWriter, r *http.Request) {
	q, err := query(r, "from", "to", "action", "actor_id", "entity", "entity_id", "request_id", "source", "limit", "offset")
	if err != nil {
		writeError(w, r, err)
		return
	}
	period, err := reportPeriod(q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := page(q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	f := repository.AuditFilter{From: period.From, To: period.To, Action: q.Get("action"), ActorID: q.Get("actor_id"), Entity: domain.AuditEntity(q.Get("entity")), EntityID: q.Get("entity_id"), RequestID: q.Get("request_id"), Source: q.Get("source"), Page: p}
	// Validate non-date filters at the boundary as well as in the service.
	if err := f.ValidateFilters(); err != nil {
		writeError(w, r, err)
		return
	}
	v, err := h.use.Audit.List(r.Context(), middleware.ActorID(r.Context()), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := dto.AuditHistoryResponse{From: v.Filter.From, To: v.Filter.To, Timezone: "Asia/Makassar", Limit: v.Filter.Page.Limit, Offset: v.Filter.Page.Offset, HasMore: v.Page.HasMore, Items: make([]dto.AuditRecordResponse, 0, len(v.Page.Items))}
	for _, e := range v.Page.Items {
		out.Items = append(out.Items, dto.AuditRecordResponse{Source: e.Source, Permission: e.Permission, AuditResponse: dto.AuditResponse{ID: e.ID, ActorID: e.ActorID, Action: e.Action, Entity: string(e.Entity), EntityID: e.EntityID, OccurredAt: e.OccurredAt, RequestID: e.RequestID, Metadata: e.Metadata}})
	}
	response.JSON(w, r, http.StatusOK, out)
}
