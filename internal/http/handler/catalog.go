package handler

import (
	"net/http"
	"time"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
)

func serviceView(r repository.ServiceRecord) dto.ServiceResponse {
	v := r.Service
	return dto.ServiceResponse{ID: v.ID, Version: r.Version, Name: v.Name, Unit: dto.PublicUnit(string(v.Unit)), UnitPrice: int64(v.UnitPrice), Minutes: int64(v.Turnaround / time.Minute), Active: v.Active, DisplayOrder: v.DisplayOrder}
}
func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	values, err := query(r, "include_inactive", "limit", "offset")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := page(values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	all := values.Get("include_inactive")
	if all != "" && all != "true" && all != "false" {
		writeError(w, r, &dto.ValidationError{Message: "Filter layanan harus true atau false."})
		return
	}
	items, err := h.use.Catalog.List(r.Context(), middleware.ActorID(r.Context()), repository.ServiceFilter{Page: p, IncludeInactive: all == "true"})
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.ServiceResponse, 0, len(items))
	for _, item := range items {
		result = append(result, serviceView(item))
	}
	response.JSON(w, r, 200, result)
}
func (h *Handler) GetService(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := query(r); err != nil {
		writeError(w, r, err)
		return
	}
	item, err := h.use.Catalog.Get(r.Context(), middleware.ActorID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 200, serviceView(item))
}
func (h *Handler) ServiceHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
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
	items, err := h.use.Catalog.History(r.Context(), middleware.ActorID(r.Context()), id, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.ServiceRevisionResponse, 0, len(items))
	for _, item := range items {
		result = append(result, dto.ServiceRevisionResponse{ServiceResponse: serviceView(item.ServiceRecord), ChangedBy: item.ActorID, ChangedAt: item.Service.UpdatedAt, Source: item.Source})
	}
	response.JSON(w, r, 200, result)
}
