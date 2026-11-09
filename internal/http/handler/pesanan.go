package handler

import (
	"bytes"
	"html/template"
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
	return dto.OrderResponse{ID: o.ID(), Customer: o.Customer().Name, Services: strings.Join(names, ", "), Total: int64(o.Total()), Status: string(o.Status()), PaymentStatus: string(o.PaymentStatus()), Version: record.Version, CreatedAt: o.CreatedAt(), EstimatedReadyAt: o.EstimatedReadyAt(), ReadyAt: nullableTime(o.ReadyAt()), Items: items, Subtotal: &subtotal, Discount: &discount}
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
	values, err := query(r, "limit", "offset", statusKey, "customer_id")
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
	if customerID != "" && !dto.Identifier(customerID) {
		writeError(w, r, &dto.ValidationError{Message: "Identitas pelanggan tidak valid."})
		return
	}
	items, err := h.use.Orders.List(r.Context(), middleware.ActorID(r.Context()), repository.OrderFilter{Page: p, Status: domain.OrderStatus(status), CustomerID: customerID})
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.OrderResponse, 0, len(items))
	for _, item := range items {
		result = append(result, dto.OrderResponse{ID: item.ID, Customer: item.CustomerName, Services: strings.Join(item.ServiceNames, ", "), Total: int64(item.Total), Status: string(item.Status), PaymentStatus: string(item.PaymentStatus), Version: item.Version, CreatedAt: item.CreatedAt, EstimatedReadyAt: item.EstimatedReadyAt})
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
	in := service.OrderInput{CustomerID: body.CustomerID, Discount: domain.Money(body.Discount), DiscountReason: body.DiscountReason, Notes: body.Notes, EstimatedReadyAt: body.EstimatedReadyAt}
	for _, item := range body.Items {
		in.Items = append(in.Items, domain.ItemRequest{ServiceID: item.ServiceID, Quantity: domain.Quantity{Pieces: item.Pieces, Grams: item.Grams}})
	}
	created, err := h.use.Orders.Create(r.Context(), middleware.ActorID(r.Context()), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/pesanan/"+url.PathEscape(created.Order.ID()))
	response.JSON(w, r, http.StatusCreated, dto.CreatedOrderResponse{Order: orderView(repository.OrderRecord{Order: created.Order, Version: 1}), TrackingToken: created.TrackingToken, TrackingExpiresAt: created.TrackingExpiresAt})
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
	item, err := h.use.Orders.ChangeStatus(r.Context(), middleware.ActorID(r.Context()), id, domain.OrderStatus(body.Status), body.Version)
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
	item, err := h.use.Orders.Cancel(r.Context(), middleware.ActorID(r.Context()), id, body.Version, body.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, orderView(item))
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
	}
	response.JSON(w, r, status, dto.PaymentResponse{ID: result.Payment.ID, Amount: int64(result.Payment.Amount), Replayed: result.Replayed})
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

var receipt = template.Must(template.New("receipt").Parse(`<!DOCTYPE html><html lang="id"><head><title>Struk {{.ID}}</title><style>body{font-family:monospace;width:300px;margin:0 auto;padding:10px;color:#000}.center{text-align:center}.line{border-bottom:1px dashed #000;margin:10px 0}@media print{body{width:100%;margin:0}}</style></head><body><div class="center"><h2>ULAUNDRY</h2><p>Perumahan Viola</p></div><div class="line"></div><p>ID Order: {{.ID}}<br>Nama: {{.Customer}}<br>Layanan: {{.Services}}<br>Mulai: {{.CreatedAt.Format "2006-01-02 15:04:05 -07:00"}}</p><h3>TOTAL: Rp {{.Total}}</h3><p>Status: {{.Status}}<br>Pembayaran: {{.PaymentStatus}}</p><button onclick="window.print()">Cetak struk</button></body></html>`))

func (h *Handler) CetakStruk(w http.ResponseWriter, r *http.Request) {
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
	if err := receipt.Execute(&body, orderView(item)); err != nil {
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
