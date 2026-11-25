package handler

import (
	"net/http"
	"strings"
	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/service"
)

func shiftTotalsView(t domain.ShiftTotals) dto.ShiftTotalsResponse {
	return dto.ShiftTotalsResponse{UnknownIn: int64(t.UnknownIn), UnknownOut: int64(t.UnknownOut), CashPayments: int64(t.CashPayments), CashRefunds: int64(t.CashRefunds), CashIn: int64(t.CashIn), CashOut: int64(t.CashOut), BankPayments: int64(t.BankPayments), BankRefunds: int64(t.BankRefunds), QRISPayments: int64(t.QRISPayments), QRISRefunds: int64(t.QRISRefunds)}
}
func shiftResponse(v service.ShiftView) dto.ShiftResponse {
	s := v.Shift
	r := dto.ShiftResponse{CashierName: v.CashierName, ID: s.ID, CashierID: s.CashierID, Status: "open", Opening: int64(s.Opening), OpeningNotes: s.OpeningNotes, OpenedAt: s.OpenedAt, ClosedBy: s.ClosedBy, ClosingNotes: s.ClosingNotes, Expected: int64(v.Expected), Totals: shiftTotalsView(v.Totals), Replayed: v.Replayed}
	if !s.IsOpen() {
		physical, difference := int64(s.Physical), int64(s.Difference)
		r.Status = "closed"
		r.ClosedAt = &s.ClosedAt
		r.Physical = &physical
		r.Difference = &difference
	}
	return r
}
func shiftEntryView(e domain.LedgerEntry, replayed bool) dto.ShiftEntryResponse {
	return dto.ShiftEntryResponse{ID: e.ID, ShiftID: e.ShiftID, OrderID: e.OrderID, PaymentID: e.PaymentID, RefundID: e.RefundID, Type: string(e.Type), Method: string(e.Method), Amount: int64(e.Amount), Notes: e.Description, CashierID: e.RecordedBy, OccurredAt: e.OccurredAt, Replayed: replayed}
}
func (h *Handler) OpenShift(w http.ResponseWriter, r *http.Request) {
	var body dto.ShiftOpenRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := orderCommandKey(w, r)
	if !ok {
		return
	}
	v, err := h.use.Shifts.Open(r.Context(), middleware.ActorID(r.Context()), service.ShiftOpenInput{Key: key, Opening: domain.Money(*body.Opening), Notes: strings.TrimSpace(body.Notes)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := 201
	if v.Replayed {
		status = 200
	}
	response.JSON(w, r, status, shiftResponse(v))
}
func (h *Handler) CloseShift(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.ShiftCloseRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := orderCommandKey(w, r)
	if !ok {
		return
	}
	v, err := h.use.Shifts.Close(r.Context(), middleware.ActorID(r.Context()), service.ShiftCloseInput{ID: id, Key: key, Physical: domain.Money(*body.Physical), Notes: strings.TrimSpace(body.Notes)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 200, shiftResponse(v))
}
func (h *Handler) MoveShiftCash(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body dto.CashMoveRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := orderCommandKey(w, r)
	if !ok {
		return
	}
	direction := domain.Debit
	if body.Direction == "out" {
		direction = domain.Credit
	}
	v, err := h.use.Shifts.Move(r.Context(), middleware.ActorID(r.Context()), service.CashMoveInput{ID: id, Key: key, Amount: domain.Money(body.Amount), Direction: direction, Notes: strings.TrimSpace(body.Notes)})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := 201
	if v.Replayed {
		status = 200
	}
	response.JSON(w, r, status, shiftEntryView(v.Movement.Entry, v.Replayed))
}
func (h *Handler) CurrentShift(w http.ResponseWriter, r *http.Request) {
	if _, err := query(r); err != nil {
		writeError(w, r, err)
		return
	}
	v, err := h.use.Shifts.Current(r.Context(), middleware.ActorID(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	if v == nil {
		response.JSON(w, r, 200, nil)
		return
	}
	response.JSON(w, r, 200, shiftResponse(*v))
}
func (h *Handler) GetShift(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.use.Shifts.Get(r.Context(), middleware.ActorID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 200, shiftResponse(v))
}
func (h *Handler) ShiftEntries(w http.ResponseWriter, r *http.Request) {
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
	rows, err := h.use.Shifts.Entries(r.Context(), middleware.ActorID(r.Context()), id, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := make([]dto.ShiftEntryResponse, 0, len(rows))
	for _, e := range rows {
		result = append(result, shiftEntryView(e, false))
	}
	response.JSON(w, r, 200, result)
}
func (h *Handler) DailyShifts(w http.ResponseWriter, r *http.Request) {
	values, err := query(r, "date")
	if err != nil {
		writeError(w, r, err)
		return
	}
	v, err := h.use.Shifts.Daily(r.Context(), middleware.ActorID(r.Context()), values.Get("date"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	result := dto.ShiftDayResponse{Date: v.Date, Shifts: make([]dto.ShiftResponse, 0, len(v.Shifts)), Totals: shiftTotalsView(v.Totals), Unassigned: shiftTotalsView(v.Unassigned)}
	for _, s := range v.Shifts {
		result.Shifts = append(result.Shifts, shiftResponse(s))
	}
	response.JSON(w, r, 200, result)
}
