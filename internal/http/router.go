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
	register("/api/login", map[string]http.HandlerFunc{"POST": h.Login})
	register("/api/session", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.CurrentSession)})
	register("/api/logout", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.Logout)})
	register("/api/tracking/{token}", map[string]http.HandlerFunc{"GET": h.Tracking})
	register("/api/pesanan", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetSemuaPesanan), "POST": m.ValidasiJWT(h.BikinPesananBaru)})
	register("/api/pesanan/status", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetPesananByStatus)})
	register("/api/pesanan/{id}", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetPesananById), "PUT": m.ValidasiJWT(h.UpdateStatusPesanan)})
	register("/api/pesanan/{id}/struk", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.CetakStruk)})
	register("/api/pesanan/{id}/cancel", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.CancelOrder)})
	register("/api/pesanan/{id}/pembayaran", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.RecordPayment)})
	register("/api/laporan/neraca", map[string]http.HandlerFunc{"GET": m.ValidasiJWT(h.GetNeraca)})
	register("/api/customers", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.CreateCustomer)})
	register("/api/services", map[string]http.HandlerFunc{"POST": m.ValidasiJWT(h.ConfigureService)})
	register("/api/services/{id}", map[string]http.HandlerFunc{"PUT": m.ValidasiJWT(h.ConfigureService)})
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response.Fail(w, r, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan.")
	})
	static := http.FileServer(assets)
	dispatch := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		static.ServeHTTP(w, r)
	}
	return middleware.RequestContext(m.BukaPintuCORS(dispatch))
}
