package http

import (
	"net/http"

	"ulaundry/internal/http/middleware"
)

// Endpoints allows routing to be checked independently of SQL and bootstrap.
type Endpoints interface {
	HealthCheck(http.ResponseWriter, *http.Request)
	GetPesananById(http.ResponseWriter, *http.Request)
	CetakStruk(http.ResponseWriter, *http.Request)
	Login(http.ResponseWriter, *http.Request)
	GetSemuaPesanan(http.ResponseWriter, *http.Request)
	GetPesananByStatus(http.ResponseWriter, *http.Request)
	BikinPesananBaru(http.ResponseWriter, *http.Request)
	UpdateStatusPesanan(http.ResponseWriter, *http.Request)
	GetNeraca(http.ResponseWriter, *http.Request)
}

// NewRouter keeps the existing methods, URLs, auth boundaries and static paths.
func NewRouter(h Endpoints, m *middleware.Middleware, assets http.FileSystem) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(assets))

	mux.HandleFunc("GET /api/health", h.HealthCheck)
	mux.HandleFunc("GET /api/pesanan/{id}", h.GetPesananById)
	mux.HandleFunc("GET /api/pesanan/{id}/struk", h.CetakStruk)
	mux.HandleFunc("POST /api/login", h.Login)

	mux.HandleFunc("GET /api/pesanan", m.ValidasiJWT(h.GetSemuaPesanan))
	mux.HandleFunc("GET /api/pesanan/status", m.ValidasiJWT(h.GetPesananByStatus))
	mux.HandleFunc("POST /api/pesanan", m.ValidasiJWT(h.BikinPesananBaru))
	mux.HandleFunc("PUT /api/pesanan/{id}", m.ValidasiJWT(h.UpdateStatusPesanan))
	mux.HandleFunc("GET /api/laporan/neraca", m.ValidasiJWT(h.GetNeraca))
	return m.BukaPintuCORS(mux.ServeHTTP)
}
