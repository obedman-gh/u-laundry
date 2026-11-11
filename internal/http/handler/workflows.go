package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/service"
)

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if h.use.Sessions == nil {
		response.Fail(w, r, 503, "service_unavailable", "Layanan autentikasi sementara tidak tersedia.")
		return
	}
	if !h.use.Sessions.SafeOrigin(r) {
		response.Fail(w, r, 403, "csrf_failed", "Origin login tidak diizinkan.")
		return
	}
	var body dto.LoginRequest
	if !decode(w, r, &body) {
		return
	}
	id := response.RequestID(r.Context())
	allowed, retry, err := h.use.Sessions.AllowLogin(r.Context(), r, body.Username, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		response.Fail(w, r, 429, "rate_limited", "Terlalu banyak percobaan login. Coba kembali setelah beberapa saat.")
		return
	}
	result, err := h.use.Auth.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		if errors.Is(err, service.ErrAuthentication) {
			if auditErr := h.use.Sessions.Audit(r.Context(), "login_failure", id); auditErr != nil {
				writeError(w, r, auditErr)
				return
			}
		}
		writeError(w, r, err)
		return
	}
	claims, err := h.use.Sessions.Start(r.Context(), result.Token, result.CredentialFingerprint, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.use.Sessions.SetCookie(w, result.Token, claims)
	response.JSON(w, r, 200, dto.LoginResponse{Name: result.Name, Message: "Login berhasil", CSRFToken: h.use.Sessions.CSRF(claims), ExpiresAt: claims.ExpiresAt.Time})
}
func (h *Handler) CurrentSession(w http.ResponseWriter, r *http.Request) {
	c := middleware.SessionClaims(r.Context())
	response.JSON(w, r, 200, map[string]any{"user_id": c.Subject, "username": c.Username, "role": c.Role, "csrf_token": h.use.Sessions.CSRF(c), "expires_at": c.ExpiresAt.Time})
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.use.Sessions.Logout(r.Context(), middleware.SessionClaims(r.Context()), response.RequestID(r.Context())); err != nil {
		writeError(w, r, err)
		return
	}
	h.use.Sessions.ClearCookie(w)
	response.JSON(w, r, 200, map[string]string{"pesan": "Sesi telah diakhiri."})
}
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := h.use.Health.Check(r.Context()); err != nil {
		response.Fail(w, r, 503, "health_unavailable", "Pemeriksaan database sementara tidak tersedia.")
		return
	}
	response.JSON(w, r, http.StatusOK, dto.HealthResponse{Status: "ok", Version: "1.0.0", Database: "connected"})
}
func (h *Handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	var body dto.CustomerRequest
	if !decode(w, r, &body) {
		return
	}
	if body.Version != 0 {
		writeError(w, r, &dto.ValidationError{Message: "Versi create harus nol."})
		return
	}
	result, err := h.use.Customers.Create(r.Context(), middleware.ActorID(r.Context()), service.CustomerInput{Name: body.Name, Contact: body.Contact, Address: body.Address, Notes: body.Notes})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, dto.CustomerResponse{ID: result.ID, Name: result.Name})
}
func (h *Handler) ConfigureService(w http.ResponseWriter, r *http.Request) {
	id := ""
	if r.Method == http.MethodPut {
		var ok bool
		id, ok = pathID(w, r)
		if !ok {
			return
		}
	}
	var body dto.ServiceRequest
	if !decode(w, r, &body) {
		return
	}
	if (id == "" && body.Version != 0) || (id != "" && body.Version < 1) {
		writeError(w, r, &dto.ValidationError{Message: "Versi konfigurasi tidak sesuai operasi create/update."})
		return
	}
	result, err := h.use.Catalog.Configure(r.Context(), middleware.ActorID(r.Context()), service.ServiceInput{ID: id, Name: body.Name, Unit: domain.Unit(dto.StoredUnit(body.Unit)), UnitPrice: domain.Money(*body.UnitPrice), Turnaround: time.Duration(body.Minutes) * time.Minute, Active: *body.Active, DisplayOrder: body.DisplayOrder, ExpectedVersion: body.Version})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if id != "" {
		status = http.StatusOK
	}
	response.JSON(w, r, status, serviceView(result))
}
func (h *Handler) GetNeraca(w http.ResponseWriter, r *http.Request) {
	values, err := query(r, "from", "to")
	if err != nil {
		writeError(w, r, err)
		return
	}
	period := service.ReportPeriod{}
	for _, field := range []struct {
		name   string
		target *time.Time
	}{{"from", &period.From}, {"to", &period.To}} {
		if raw := values.Get(field.name); raw != "" {
			at, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeError(w, r, &dto.ValidationError{Message: "Tanggal harus RFC3339 dengan zona waktu."})
				return
			}
			*field.target = at.UTC().Truncate(time.Microsecond)
		}
	}
	if period.From.IsZero() != period.To.IsZero() || (!period.From.IsZero() && !period.To.After(period.From)) {
		writeError(w, r, &dto.ValidationError{Message: "Awal dan akhir periode harus diisi berpasangan dan berurutan."})
		return
	}
	result, err := h.use.Reports.Generate(r.Context(), middleware.ActorID(r.Context()), period)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, dto.ReportResponse{ID: result.ID, From: result.From, To: result.To, Inflow: int64(result.Inflow), Outflow: int64(result.Outflow), Balance: int64(result.Balance)})
}
