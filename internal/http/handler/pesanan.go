package handler

import (
	"bytes"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
	"ulaundry/internal/telemetry"
)

func orderView(record repository.OrderRecord) dto.OrderResponse {
	o := record.Order
	names := make([]string, 0, len(o.Items()))
	items := make([]dto.OrderItemResponse, 0, len(o.Items()))
	for _, item := range o.Items() {
		names = append(names, item.ServiceName)
		items = append(items, dto.OrderItemResponse{ServiceID: item.ServiceID, Name: item.ServiceName, Unit: dto.PublicUnit(string(item.Unit)), UnitPrice: int64(item.UnitPrice), Pieces: item.Quantity.Pieces, Grams: item.Quantity.Grams, LineTotal: int64(item.LineTotal)})
	}
	subtotal, discount := int64(o.Subtotal()), int64(o.Discount().Amount)
	return dto.OrderResponse{Paid: int64(o.Paid()), Balance: int64(o.Balance()), Notes: o.Notes(), CancellationReason: o.CancellationReason(), ID: o.ID(), Customer: o.Customer().Name, Services: strings.Join(names, ", "), Total: int64(o.Total()), Status: string(o.Status()), PaymentStatus: string(o.PaymentStatus()), Version: record.Version, CreatedAt: o.CreatedAt(), EstimatedReadyAt: o.EstimatedReadyAt(), ReadyAt: nullableTime(o.ReadyAt()), ProcessingAt: nullableTime(o.ProcessingAt()), CollectedAt: nullableTime(o.CollectedAt()), CancelledAt: nullableTime(o.CancelledAt()), Replayed: record.Replayed, AppliedVersion: record.AppliedVersion, Items: items, Subtotal: &subtotal, Discount: &discount}
}
func (h *Handler) GetSemuaPesanan(w http.ResponseWriter, r *http.Request) { h.list(w, r, false) }
func (h *Handler) GetPesananByStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Deprecation", "@"+strconv.FormatInt(time.Date(2026, time.November, 7, 0, 0, 0, 0, time.UTC).Unix(), 10))
	w.Header().Set("Link", `</api/pesanan>; rel="successor-version"`)
	h.list(w, r, true)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request, legacy bool) {
	statusKey := "status"
	if legacy {
		statusKey = "q"
	}
	values, err := query(r, "limit", "offset", statusKey, "customer_id", "search")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := page(values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := values.Get(statusKey)
	if (legacy && status == "") || (status != "" && !dto.Status(status)) {
		writeError(w, r, &dto.ValidationError{Message: "Filter status tidak valid."})
		return
	}
	customerID := values.Get("customer_id")
	search, err := domain.OrderSearch(values.Get("search"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	if customerID != "" && !dto.Identifier(customerID) {
		writeError(w, r, &dto.ValidationError{Message: "Identitas pelanggan tidak valid."})
		return
	}
	items, err := h.use.Orders.List(r.Context(), middleware.ActorID(r.Context()), repository.OrderFilter{Page: p, Status: domain.OrderStatus(status), CustomerID: customerID, Search: search})
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.OrderResponse, 0, len(items))
	for _, item := range items {
		result = append(result, dto.OrderResponse{Paid: int64(item.Paid), Balance: int64(item.Total - item.Paid), ID: item.ID, Customer: item.CustomerName, Services: strings.Join(item.ServiceNames, ", "), Total: int64(item.Total), Status: string(item.Status), PaymentStatus: string(item.PaymentStatus), Version: item.Version, CreatedAt: item.CreatedAt, EstimatedReadyAt: item.EstimatedReadyAt})
	}
	response.JSON(w, r, http.StatusOK, result)
}
func (h *Handler) GetPesananById(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := h.use.Orders.Get(r.Context(), middleware.ActorID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, orderView(item))
}
func (h *Handler) BikinPesananBaru(w http.ResponseWriter, r *http.Request) {
	var body dto.OrderRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := orderCommandKey(w, r)
	if !ok {
		return
	}
	in := service.OrderInput{IdempotencyKey: key, CustomerID: body.CustomerID, Discount: domain.Money(body.Discount), DiscountReason: body.DiscountReason, Notes: body.Notes, EstimatedReadyAt: body.EstimatedReadyAt}
	for _, item := range body.Items {
		in.Items = append(in.Items, domain.ItemRequest{ServiceID: item.ServiceID, Quantity: domain.Quantity{Pieces: item.Pieces, Grams: item.Grams}})
	}
	created, err := h.use.Orders.Create(r.Context(), middleware.ActorID(r.Context()), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/pesanan/"+url.PathEscape(created.Order.ID()))
	status := http.StatusCreated
	if created.Replayed {
		status = http.StatusOK
	} else {
		h.use.Metrics.Business(telemetry.OrderCreated)
	}
	response.JSON(w, r, status, dto.CreatedOrderResponse{Order: orderView(repository.OrderRecord{Order: created.Order, Version: created.Version, AppliedVersion: 1, Replayed: created.Replayed}), TrackingToken: created.TrackingToken, TrackingExpiresAt: nullableTime(created.TrackingExpiresAt), Replayed: created.Replayed})
}
func (h *Handler) UpdateStatusPesanan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.StatusRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := orderCommandKey(w, r)
	if !ok {
		return
	}
	item, err := h.use.Orders.ChangeStatus(r.Context(), middleware.ActorID(r.Context()), id, domain.OrderStatus(body.Status), body.Version, key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, orderView(item))
}
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.CancelRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := orderCommandKey(w, r)
	if !ok {
		return
	}
	item, err := h.use.Orders.Cancel(r.Context(), middleware.ActorID(r.Context()), id, body.Version, body.Reason, key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, orderView(item))
}
func orderCommandKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !dto.Identifier(keys[0]) {
		writeError(w, r, &dto.ValidationError{Message: "Satu Idempotency-Key dengan format ID yang sah wajib diisi."})
		return "", false
	}
	return keys[0], true
}
func (h *Handler) OrderHistory(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.use.Orders.History(r.Context(), middleware.ActorID(r.Context()), id, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.OrderStatusEventResponse, 0, len(items))
	for _, e := range items {
		result = append(result, dto.OrderStatusEventResponse{Version: e.Version, From: string(e.From), To: string(e.To), ActorID: e.ActorID, OccurredAt: e.OccurredAt, Source: e.Source})
	}
	response.JSON(w, r, http.StatusOK, result)
}
func (h *Handler) RecordPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.PaymentRequest
	if !decode(w, r, &body) {
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !dto.Identifier(keys[0]) {
		writeError(w, r, &dto.ValidationError{Message: "Satu Idempotency-Key dengan format ID yang sah wajib diisi."})
		return
	}
	result, err := h.use.Payments.Record(r.Context(), middleware.ActorID(r.Context()), service.PaymentInput{OrderID: id, IdempotencyKey: keys[0], Amount: domain.Money(body.Amount), Method: domain.PaymentMethod(body.Method)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	} else {
		h.use.Metrics.Business(telemetry.PaymentRecorded)
	}
	response.JSON(w, r, status, paymentView(result.Payment, result.Replayed))
}
func (h *Handler) Tracking(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	token := r.PathValue("token")
	if !dto.TrackingToken(token) {
		response.Fail(w, r, 404, "not_found", "Data tidak ditemukan.")
		return
	}
	view, err := h.use.Tracking.Get(r.Context(), token)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, dto.TrackingResponse{Code: view.Code, Status: string(view.Status), EstimatedReadyAt: view.EstimatedReadyAt, Message: view.Message})
}

func (h *Handler) CetakStruk(w http.ResponseWriter, r *http.Request) {
	values, err := query(r, "paper")
	if err != nil {
		writeError(w, r, err)
		return
	}
	paper := values.Get("paper")
	if paper == "" {
		paper = "58"
	}
	if paper != "58" && paper != "80" {
		writeError(w, r, &dto.ValidationError{Message: "Lebar kertas harus 58 atau 80 mm."})
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := h.use.Orders.Get(r.Context(), middleware.ActorID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body bytes.Buffer
	if err := receipt.Execute(&body, receiptData{OrderResponse: orderView(item), Paper: paper}); err != nil {
		writeError(w, r, err)
		return
	}
	response.Prepare(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
	if r.Method != http.MethodHead {
		_, _ = w.Write(body.Bytes())
	}
}
