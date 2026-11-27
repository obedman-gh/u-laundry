package handler

import (
	"context"
	"net/http"
	"time"

	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/telemetry"
)

func (h *Handler) Liveness(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, map[string]string{"status": "alive"})
}
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	if h.use.Draining != nil && h.use.Draining() {
		telemetry.SetError(r.Context(), "shutdown")
		response.Fail(w, r, 503, "not_ready", "Layanan sedang dihentikan.")
		return
	}
	timeout := h.use.ReadinessTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	var err error
	if h.use.Health != nil {
		err = h.use.Health.Check(ctx)
	}
	if h.use.Health == nil || err != nil || ctx.Err() != nil {
		if ctx.Err() != nil {
			class := "timeout"
			if ctx.Err() == context.Canceled {
				class = "canceled"
			}
			telemetry.SetError(r.Context(), class)
		}
		response.Fail(w, r, 503, "health_unavailable", "Pemeriksaan database sementara tidak tersedia.")
		return
	}
	if h.use.Draining != nil && h.use.Draining() {
		telemetry.SetError(r.Context(), "shutdown")
		response.Fail(w, r, 503, "not_ready", "Layanan sedang dihentikan.")
		return
	}
	response.JSON(w, r, 200, dto.HealthResponse{Status: "ok", Version: "1.0.0", Database: "connected"})
}
func (h *Handler) OperationalMetrics(w http.ResponseWriter, r *http.Request) {
	if h.use.Operations == nil || h.use.Metrics == nil {
		response.Fail(w, r, 503, "service_unavailable", "Diagnostik sementara tidak tersedia.")
		return
	}
	if err := h.use.Operations.Authorize(r.Context(), middleware.ActorID(r.Context())); err != nil {
		writeError(w, r, err)
		return
	}
	var pool *telemetry.Pool
	if h.use.PoolStats != nil {
		v := h.use.PoolStats()
		pool = &v
	}
	response.JSON(w, r, 200, struct {
		telemetry.Snapshot
		Pool *telemetry.Pool `json:"db_pool"`
	}{h.use.Metrics.Snapshot(), pool})
}
