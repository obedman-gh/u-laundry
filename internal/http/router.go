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

	mux.HandleFunc("GET /api/health", m.BukaPintuCORS(h.HealthCheck))
	mux.HandleFunc("GET /api/pesanan/{id}", m.BukaPintuCORS(h.GetPesananById))
	mux.HandleFunc("GET /api/pesanan/{id}/struk", m.BukaPintuCORS(h.CetakStruk))
	mux.HandleFunc("POST /api/login", m.BukaPintuCORS(h.Login))

	mux.HandleFunc("GET /api/pesanan", m.BukaPintuCORS(m.ValidasiJWT(h.GetSemuaPesanan)))
	mux.HandleFunc("GET /api/pesanan/status", m.BukaPintuCORS(m.ValidasiJWT(h.GetPesananByStatus)))
	mux.HandleFunc("POST /api/pesanan", m.BukaPintuCORS(m.ValidasiJWT(h.BikinPesananBaru)))
	mux.HandleFunc("PUT /api/pesanan/{id}", m.BukaPintuCORS(m.ValidasiJWT(h.UpdateStatusPesanan)))
	mux.HandleFunc("GET /api/laporan/neraca", m.BukaPintuCORS(m.ValidasiJWT(h.GetNeraca)))
	return mux
}
