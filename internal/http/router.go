package http

import (
	"net/http"
	"sort"
	"strings"
	"ulaundry/internal/http/response"

	"ulaundry/internal/http/middleware"
)

// Endpoints allows routing to be checked independently of SQL and bootstrap.
type Endpoints interface {
	AuditEvents(http.ResponseWriter, *http.Request)
	DashboardReport(http.ResponseWriter, *http.Request)
	ReportOrders(http.ResponseWriter, *http.Request)
	ReportLedger(http.ResponseWriter, *http.Request)
	OpenShift(http.ResponseWriter, *http.Request)
	CloseShift(http.ResponseWriter, *http.Request)
	MoveShiftCash(http.ResponseWriter, *http.Request)
	CurrentShift(http.ResponseWriter, *http.Request)
	GetShift(http.ResponseWriter, *http.Request)
	DailyShifts(http.ResponseWriter, *http.Request)
	ShiftEntries(http.ResponseWriter, *http.Request)
	RefundPayment(http.ResponseWriter, *http.Request)
	ListPayments(http.ResponseWriter, *http.Request)
	ListRefunds(http.ResponseWriter, *http.Request)
	OrderHistory(http.ResponseWriter, *http.Request)
	ListServices(http.ResponseWriter, *http.Request)
	GetService(http.ResponseWriter, *http.Request)
	ServiceHistory(http.ResponseWriter, *http.Request)
	SearchCustomers(http.ResponseWriter, *http.Request)
	GetCustomer(http.ResponseWriter, *http.Request)
	UpdateCustomer(http.ResponseWriter, *http.Request)
	CustomerOrders(http.ResponseWriter, *http.Request)
	RotateTracking(http.ResponseWriter, *http.Request)
	ListStaff(http.ResponseWriter, *http.Request)
	CreateStaff(http.ResponseWriter, *http.Request)
	ChangeStaffActive(http.ResponseWriter, *http.Request)
	ChangeStaffRole(http.ResponseWriter, *http.Request)
	ResetStaffPassword(http.ResponseWriter, *http.Request)
	StaffAudit(http.ResponseWriter, *http.Request)
	CurrentSession(http.ResponseWriter, *http.Request)
	Logout(http.ResponseWriter, *http.Request)
	HealthCheck(http.ResponseWriter, *http.Request)
	GetPesananById(http.ResponseWriter, *http.Request)
	CetakStruk(http.ResponseWriter, *http.Request)
	Login(http.ResponseWriter, *http.Request)
	GetSemuaPesanan(http.ResponseWriter, *http.Request)
	GetPesananByStatus(http.ResponseWriter, *http.Request)
	BikinPesananBaru(http.ResponseWriter, *http.Request)
	UpdateStatusPesanan(http.ResponseWriter, *http.Request)
	GetNeraca(http.ResponseWriter, *http.Request)
	CreateCustomer(http.ResponseWriter, *http.Request)
	ConfigureService(http.ResponseWriter, *http.Request)
	CancelOrder(http.ResponseWriter, *http.Request)
	RecordPayment(http.ResponseWriter, *http.Request)
	Tracking(http.ResponseWriter, *http.Request)
}

// NewRouter keeps API routing separate from the static file namespace.
func NewRouter(h Endpoints, m *middleware.Middleware, assets http.FileSystem) http.Handler {
	api := http.NewServeMux()
	register := func(path string, methods map[string]http.HandlerFunc) {
		if get, ok := methods[http.MethodGet]; ok {
			methods[http.MethodHead] = get
		}
		allowed := make([]string, 0, len(methods))
		for method := range methods {
			allowed = append(allowed, method)
		}
		sort.Strings(allowed)
		api.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if serve, ok := methods[r.Method]; ok {
				serve(w, r)
				return
			}
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			response.Fail(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Metode permintaan tidak diizinkan untuk endpoint ini.")
		})
	}
	register("/api/health", map[string]http.HandlerFunc{"GET": h.HealthCheck})
	register("/api/audit/events", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.AuditEvents)})
	register("/api/laporan/dashboard", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.DashboardReport)})
	register("/api/laporan/orders", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.ReportOrders)})
	register("/api/laporan/ledger", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.ReportLedger)})
	register("/api/shifts", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.OpenShift)})
	register("/api/shifts/current", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.CurrentShift)})
	register("/api/shifts/daily", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.DailyShifts)})
	register("/api/shifts/{id}", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetShift)})
	register("/api/shifts/{id}/close", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.CloseShift)})
	register("/api/shifts/{id}/cash", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.MoveShiftCash)})
	register("/api/shifts/{id}/entries", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.ShiftEntries)})
	register("/api/login", map[string]http.HandlerFunc{"POST": h.Login})
	register("/api/session", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.CurrentSession)})
	register("/api/logout", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.Logout)})
	register("/api/tracking/{token}", map[string]http.HandlerFunc{"GET": m.TrackingLimit(h.Tracking)})
	register("/api/pesanan", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetSemuaPesanan), "POST": m.ValidasiJWT(h.BikinPesananBaru)})
	register("/api/pesanan/status", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetPesananByStatus)})
	register("/api/pesanan/{id}", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetPesananById), "PUT": m.ValidasiJWT(h.UpdateStatusPesanan)})
	register("/api/pesanan/{id}/struk", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.CetakStruk)})
	register("/api/pesanan/{id}/tracking", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.RotateTracking)})
	register("/api/pesanan/{id}/cancel", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.CancelOrder)})
	register("/api/pesanan/{id}/pembayaran", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.RecordPayment), "GET": m.ValidasiJWT(h.ListPayments)})
	register("/api/pesanan/{id}/refunds", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.ListRefunds), "POST": m.ValidasiJWT(h.RefundPayment)})
	register("/api/laporan/neraca", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetNeraca)})
	register("/api/customers", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.SearchCustomers), "POST": m.ValidasiJWT(h.CreateCustomer)})
	register("/api/customers/{id}", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetCustomer), "PUT": m.ValidasiJWT(h.UpdateCustomer)})
	register("/api/customers/{id}/orders", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.CustomerOrders)})
	register("/api/services", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.ListServices), "POST": m.ValidasiJWT(h.ConfigureService)})
	register("/api/services/{id}", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetService), "PUT": m.ValidasiJWT(h.ConfigureService)})
	register("/api/services/{id}/history", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.ServiceHistory)})
	register("/api/pesanan/{id}/history", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.OrderHistory)})
	register("/api/staff", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.ListStaff), "POST": m.ValidasiJWT(h.CreateStaff)})
	register("/api/staff/{id}/active", map[string]http.HandlerFunc{"PUT": m.ValidasiJWT(h.ChangeStaffActive)})
	register("/api/staff/{id}/role", map[string]http.HandlerFunc{"PUT": m.ValidasiJWT(h.ChangeStaffRole)})
	register("/api/staff/{id}/reset-password", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.ResetStaffPassword)})
	register("/api/audit", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.StaffAudit)})
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response.Fail(w, r, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan.")
	})
	static := publicAssets{assets}
	dispatch := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			response.Fail(w, r, 405, "method_not_allowed", "Metode permintaan tidak diizinkan untuk aset ini.")
			return
		}
		static.ServeHTTP(w, r)
	}
	return middleware.RequestContext(m.SecurityHeaders(m.BukaPintuCORS(middleware.RequestLimits(http.HandlerFunc(dispatch)).ServeHTTP)))
}
