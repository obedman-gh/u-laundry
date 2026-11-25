package handler

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

func reportPeriod(values url.Values) (service.ReportPeriod, error) {
	v := service.ReportPeriod{}
	for _, field := range []struct {
		name   string
		target *time.Time
	}{{"from", &v.From}, {"to", &v.To}} {
		if raw := values.Get(field.name); raw != "" {
			at, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return v, &dto.ValidationError{Message: "Tanggal harus RFC3339 dengan zona waktu."}
			}
			*field.target = at.UTC().Truncate(time.Microsecond)
		}
	}
	if v.From.IsZero() != v.To.IsZero() || (!v.From.IsZero() && (!v.To.After(v.From) || v.To.Sub(v.From) > 366*24*time.Hour)) {
		return v, &dto.ValidationError{Message: "Isi periode berurutan maksimal 366 hari dengan zona waktu."}
	}
	return v, nil
}
func reportMeta(v service.ReportMeta) dto.ReportMeta {
	return dto.ReportMeta{ID: v.ID, From: v.Window.From, To: v.Window.To, AsOf: v.Window.AsOf, Timezone: "Asia/Makassar"}
}
func (h *Handler) DashboardReport(w http.ResponseWriter, r *http.Request) {
	q, err := query(r, "from", "to")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := reportPeriod(q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	v, err := h.use.Reports.Dashboard(r.Context(), middleware.ActorID(r.Context()), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	m := v.Metrics
	result := dto.DashboardResponse{
		ReportMeta: reportMeta(v.ReportMeta),
		Orders:     dto.ReportOrders{Created: m.Orders.Created, Received: m.Orders.Received, Processing: m.Orders.Processing, Ready: m.Orders.Ready, Overdue: m.Orders.Overdue, ReceivableOrders: m.Orders.ReceivableOrders, Receivable: int64(m.Orders.Receivable)},
		Money:      dto.ReportMoney{Payments: int64(m.Money.Payments), Refunds: int64(m.Money.Refunds), Net: int64(m.Money.Net), Expenses: int64(m.Money.Expenses), CashIn: int64(m.Money.CashIn), CashOut: int64(m.Money.CashOut), UnassignedPayments: int64(m.Money.UnassignedPayments), UnassignedRefunds: int64(m.Money.UnassignedRefunds)},
		Shifts:     dto.ReportShifts{Opened: m.Shifts.Opened, Closed: m.Shifts.Closed, Active: m.Shifts.Active, System: int64(m.Shifts.System), Physical: int64(m.Shifts.Physical), Difference: int64(m.Shifts.Difference)},
		Methods:    make([]dto.ReportMethod, 0, len(m.Methods)), Popular: make([]dto.ReportPopular, 0, len(m.Popular)),
	}
	for _, x := range m.Methods {
		result.Methods = append(result.Methods, dto.ReportMethod{Method: x.Method, Payments: int64(x.Payments), Refunds: int64(x.Refunds)})
	}
	for _, x := range m.Popular {
		result.Popular = append(result.Popular, dto.ReportPopular{ID: x.ID, Name: x.Name, Unit: dto.PublicUnit(string(x.Unit)), Orders: x.Orders, Pieces: x.Pieces, Grams: x.Grams, Gross: int64(x.Gross)})
	}
	response.JSON(w, r, 200, result)
}

func reportDetailQuery(r *http.Request, orders bool) (url.Values, service.ReportPeriod, repository.Page, error) {
	allowed := []string{"from", "to", "limit", "offset", "format"}
	if orders {
		allowed = append(allowed, "kind", "status")
	}
	q, err := query(r, allowed...)
	if err != nil {
		return nil, service.ReportPeriod{}, repository.Page{}, err
	}
	p, err := reportPeriod(q)
	if err != nil {
		return nil, p, repository.Page{}, err
	}
	page, err := page(q)
	if err != nil {
		return nil, p, page, err
	}
	if f := q.Get("format"); f != "" && f != "json" && f != "csv" {
		return nil, p, page, &dto.ValidationError{Message: "Format harus json atau csv."}
	}
	if orders {
		if kind := q.Get("kind"); kind != "" && kind != "created" && kind != "receivable" && kind != "overdue" {
			return nil, p, page, &dto.ValidationError{Message: "Jenis laporan pesanan tidak dikenal."}
		}
		if status := domain.OrderStatus(q.Get("status")); status != "" && !status.Valid() {
			return nil, p, page, &dto.ValidationError{Message: "Status pesanan tidak dikenal."}
		}
	}
	return q, p, page, nil
}
func (h *Handler) ReportOrders(w http.ResponseWriter, r *http.Request) {
	q, period, p, err := reportDetailQuery(r, true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	v, err := h.use.Reports.OrderDetails(r.Context(), middleware.ActorID(r.Context()), period, repository.ReportOrderFilter{Page: p, Kind: q.Get("kind"), Status: domain.OrderStatus(q.Get("status"))})
	if err != nil {
		writeError(w, r, err)
		return
	}
	items := make([]dto.ReportOrder, 0, len(v.Rows.Items))
	for _, x := range v.Rows.Items {
		items = append(items, dto.ReportOrder{ID: x.ID, CustomerName: x.CustomerName, Status: string(x.Status), Total: int64(x.Total), Paid: int64(x.Paid), Due: int64(x.Due), CreatedAt: x.CreatedAt, EstimatedReadyAt: x.EstimatedReadyAt})
	}
	if q.Get("format") == "csv" {
		rows := [][]string{{"id", "pelanggan", "status", "total_rupiah", "terbayar_rupiah", "piutang_rupiah", "dibuat_wita", "estimasi_selesai_wita"}}
		for _, x := range items {
			rows = append(rows, []string{csvText(x.ID), csvText(x.CustomerName), csvText(x.Status), integer(x.Total), integer(x.Paid), integer(x.Due), csvTime(x.CreatedAt), csvTime(x.EstimatedReadyAt)})
		}
		reportCSV(w, r, "pesanan", v.ReportMeta, v.Filter.Page, v.Rows.HasMore, rows)
		return
	}
	response.JSON(w, r, 200, dto.ReportPage[dto.ReportOrder]{ReportMeta: reportMeta(v.ReportMeta), Kind: v.Filter.Kind, Limit: v.Filter.Page.Limit, Offset: v.Filter.Page.Offset, HasMore: v.Rows.HasMore, Items: items})
}
func (h *Handler) ReportLedger(w http.ResponseWriter, r *http.Request) {
	q, period, p, err := reportDetailQuery(r, false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	v, err := h.use.Reports.LedgerDetails(r.Context(), middleware.ActorID(r.Context()), period, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items := make([]dto.ReportLedger, 0, len(v.Rows.Items))
	for _, x := range v.Rows.Items {
		items = append(items, dto.ReportLedger{ID: x.ID, OrderID: x.OrderID, PaymentID: x.PaymentID, RefundID: x.RefundID, ShiftID: x.ShiftID, Type: string(x.Type), Method: string(x.Method), Amount: int64(x.Amount), Description: x.Description, RecordedBy: x.RecordedBy, OccurredAt: x.OccurredAt})
	}
	if q.Get("format") == "csv" {
		rows := [][]string{{"id", "pesanan_id", "pembayaran_id", "refund_id", "shift_id", "arah", "metode", "nominal_rupiah", "keterangan", "pencatat_id", "waktu_wita"}}
		for _, x := range items {
			rows = append(rows, []string{csvText(x.ID), csvText(x.OrderID), csvText(x.PaymentID), csvText(x.RefundID), csvText(x.ShiftID), csvText(x.Type), csvText(x.Method), integer(x.Amount), csvText(x.Description), csvText(x.RecordedBy), csvTime(x.OccurredAt)})
		}
		reportCSV(w, r, "buku-kas", v.ReportMeta, v.Page, v.Rows.HasMore, rows)
		return
	}
	response.JSON(w, r, 200, dto.ReportPage[dto.ReportLedger]{ReportMeta: reportMeta(v.ReportMeta), Kind: "ledger", Limit: v.Page.Limit, Offset: v.Page.Offset, HasMore: v.Rows.HasMore, Items: items})
}
func integer(v int64) string      { return strconv.FormatInt(v, 10) }
func csvTime(at time.Time) string { return at.In(time.FixedZone("WITA", 8*60*60)).Format(time.RFC3339) }

// CSV quoting alone does not stop spreadsheet formulas. Inspect the first
// non-space/control rune and protect leading control characters as well.
func csvText(v string) string {
	trimmed := strings.TrimLeftFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	if strings.ContainsAny(v, "\t\r\n") || (len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + v
	}
	return v
}
func reportCSV(w http.ResponseWriter, r *http.Request, name string, meta service.ReportMeta, p repository.Page, more bool, rows [][]string) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.UseCRLF = true
	if err := writer.WriteAll(rows); err != nil {
		writeError(w, r, err)
		return
	}
	response.Prepare(w, r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="ulaundry-`+name+`-offset-`+strconv.Itoa(p.Offset)+`.csv"`)
	w.Header().Set("X-Report-ID", meta.ID)
	w.Header().Set("X-Report-From", meta.Window.From.Format(time.RFC3339))
	w.Header().Set("X-Report-To", meta.Window.To.Format(time.RFC3339))
	w.Header().Set("X-Report-As-Of", meta.Window.AsOf.Format(time.RFC3339))
	w.Header().Set("X-Report-Limit", strconv.Itoa(p.Limit))
	w.Header().Set("X-Report-Offset", strconv.Itoa(p.Offset))
	w.Header().Set("X-Report-Has-More", strconv.FormatBool(more))
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(buf.Bytes())
	}
}
