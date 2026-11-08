package handler

import (
	"context"
	"net/http"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type StaffUseCase interface {
	List(context.Context, string, repository.Page) ([]repository.StaffRecord, error)
	Create(context.Context, string, service.StaffInput) (repository.StaffRecord, error)
	Change(context.Context, string, string, string, service.StaffChange) (repository.StaffRecord, error)
	Audit(context.Context, string, domain.AuditEntity, string, repository.Page) ([]domain.AuditEvent, error)
}

func staffView(v repository.StaffRecord) dto.StaffResponse {
	return dto.StaffResponse{ID: v.Staff.ID, Username: v.Staff.Username, Name: v.Staff.Name, Role: string(v.Staff.Role), Active: v.Staff.Active, Version: v.Version, CreatedAt: v.Staff.CreatedAt, UpdatedAt: v.Staff.UpdatedAt}
}
func (h *Handler) ListStaff(w http.ResponseWriter, r *http.Request) {
	values, err := query(r, "limit", "offset")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := page(values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	records, err := h.use.Staff.List(r.Context(), middleware.ActorID(r.Context()), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]dto.StaffResponse, 0, len(records))
	for _, v := range records {
		out = append(out, staffView(v))
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) CreateStaff(w http.ResponseWriter, r *http.Request) {
	var body dto.StaffRequest
	if !decode(w, r, &body) {
		return
	}
	v, err := h.use.Staff.Create(r.Context(), middleware.ActorID(r.Context()), service.StaffInput{Username: body.Username, Name: body.Name, Role: domain.Role(body.Role), Password: body.Password})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 201, staffView(v))
}
func (h *Handler) ChangeStaffActive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.StaffActiveRequest
	if !decode(w, r, &body) {
		return
	}
	h.changeStaff(w, r, id, "active", service.StaffChange{Version: body.Version, Active: *body.Active})
}
func (h *Handler) ChangeStaffRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.StaffRoleRequest
	if !decode(w, r, &body) {
		return
	}
	h.changeStaff(w, r, id, "role", service.StaffChange{Version: body.Version, Role: domain.Role(body.Role)})
}
func (h *Handler) ResetStaffPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.StaffPasswordRequest
	if !decode(w, r, &body) {
		return
	}
	h.changeStaff(w, r, id, "password", service.StaffChange{Version: body.Version, Password: body.Password})
}
func (h *Handler) changeStaff(w http.ResponseWriter, r *http.Request, id, operation string, in service.StaffChange) {
	v, err := h.use.Staff.Change(r.Context(), middleware.ActorID(r.Context()), id, operation, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if operation == "password" && id == middleware.ActorID(r.Context()) && h.use.Sessions != nil {
		h.use.Sessions.ClearCookie(w)
	}
	response.JSON(w, r, 200, staffView(v))
}
func (h *Handler) StaffAudit(w http.ResponseWriter, r *http.Request) {
	values, err := query(r, "entity", "entity_id", "limit", "offset")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := page(values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if !dto.Identifier(values.Get("entity_id")) {
		writeError(w, r, &dto.ValidationError{Message: "Identitas objek audit wajib diisi."})
		return
	}
	records, err := h.use.Staff.Audit(r.Context(), middleware.ActorID(r.Context()), domain.AuditEntity(values.Get("entity")), values.Get("entity_id"), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]dto.AuditResponse, 0, len(records))
	for _, v := range records {
		out = append(out, dto.AuditResponse{ID: v.ID, ActorID: v.ActorID, Action: string(v.Action), Entity: string(v.Entity), EntityID: v.EntityID, OccurredAt: v.OccurredAt})
	}
	response.JSON(w, r, 200, out)
}
