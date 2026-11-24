package handler

import (
	"net/http"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/service"
)

func paymentView(p domain.Payment, replayed bool) dto.PaymentResponse {
	return dto.PaymentResponse{ID: p.ID, OrderID: p.OrderID, Amount: int64(p.Amount), Method: string(p.Method), ReceivedAt: p.ReceivedAt, CashierID: p.RecordedBy, Status: string(p.Status), IdempotencyKey: p.IdempotencyKey, Replayed: replayed}
}
func refundView(r domain.Refund, replayed bool) dto.RefundResponse {
	return dto.RefundResponse{ID: r.ID, OrderID: r.OrderID, PaymentID: r.PaymentID, Amount: int64(r.Amount), Method: string(r.Method), RefundedAt: r.RefundedAt, CashierID: r.RecordedBy, Status: "posted", IdempotencyKey: r.IdempotencyKey, Reason: r.Reason, Replayed: replayed}
}
func (h *Handler) RefundPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.RefundRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := orderCommandKey(w, r)
	if !ok {
		return
	}
	result, err := h.use.Payments.Refund(r.Context(), middleware.ActorID(r.Context()), service.RefundInput{OrderID: id, PaymentID: body.PaymentID, IdempotencyKey: key, Reason: body.Reason})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, refundView(result.Refund, result.Replayed))
}
func (h *Handler) ListPayments(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.use.Payments.List(r.Context(), middleware.ActorID(r.Context()), id, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.PaymentResponse, 0, len(items))
	for _, item := range items {
		result = append(result, paymentView(item, false))
	}
	response.JSON(w, r, http.StatusOK, result)
}
func (h *Handler) ListRefunds(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.use.Payments.Refunds(r.Context(), middleware.ActorID(r.Context()), id, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.RefundResponse, 0, len(items))
	for _, item := range items {
		result = append(result, refundView(item, false))
	}
	response.JSON(w, r, http.StatusOK, result)
}
