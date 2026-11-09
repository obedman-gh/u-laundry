package handler

import (
	"net/http"
	"strings"
	"time"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

func customerDetail(r repository.CustomerRecord) dto.CustomerDetail {
	c := r.Customer
	return dto.CustomerDetail{CustomerSummary: dto.CustomerSummary{ID: c.ID, Name: c.Name, Contact: c.Contact}, Address: c.Address, Notes: c.Notes, Version: r.Version}
}
func (h *Handler) SearchCustomers(w http.ResponseWriter, r *http.Request) {
	values, err := query(r, "q", "limit", "offset")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := page(values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := h.use.Customers.Search(r.Context(), middleware.ActorID(r.Context()), values.Get("q"), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.CustomerSummary, 0, len(items))
	for _, c := range items {
		result = append(result, dto.CustomerSummary{ID: c.ID, Name: c.Name, Contact: c.Contact})
	}
	response.JSON(w, r, 200, result)
}
func (h *Handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := h.use.Customers.Get(r.Context(), middleware.ActorID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 200, customerDetail(item))
}
func (h *Handler) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.CustomerRequest
	if !decode(w, r, &body) {
		return
	}
	if body.Version < 1 {
		writeError(w, r, &dto.ValidationError{Message: "Versi positif wajib diisi."})
		return
	}
	item, err := h.use.Customers.Update(r.Context(), middleware.ActorID(r.Context()), id, service.CustomerInput{Name: body.Name, Contact: body.Contact, Address: body.Address, Notes: body.Notes}, body.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 200, customerDetail(item))
}
func (h *Handler) CustomerOrders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := h.use.Customers.Get(r.Context(), middleware.ActorID(r.Context()), id); err != nil {
		writeError(w, r, err)
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
	items, err := h.use.Orders.List(r.Context(), middleware.ActorID(r.Context()), repository.OrderFilter{CustomerID: id, Page: p})
	if err != nil {
		writeError(w, r, err)
		return
	}
	// Reuse internal order summaries; never include tokens, ledger or customer notes.
	result := make([]dto.OrderResponse, 0, len(items))
	for _, item := range items {
		result = append(result, dto.OrderResponse{ID: item.ID, Customer: item.CustomerName, Services: strings.Join(item.ServiceNames, ", "), Total: int64(item.Total), Status: string(item.Status), PaymentStatus: string(item.PaymentStatus), Version: item.Version, CreatedAt: item.CreatedAt, EstimatedReadyAt: item.EstimatedReadyAt})
	}
	response.JSON(w, r, 200, result)
}
func (h *Handler) RotateTracking(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.TrackingRotationRequest
	if !decode(w, r, &body) {
		return
	}
	result, err := h.use.Orders.RotateTracking(r.Context(), middleware.ActorID(r.Context()), id, body.Version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 200, struct {
		Token     string    `json:"tracking_token"`
		ExpiresAt time.Time `json:"tracking_expires_at"`
		Version   int64     `json:"version"`
	}{result.Token, result.ExpiresAt, result.Version})
}
