package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"ulaundry/internal/http/response"

	"errors"
	"ulaundry/internal/security"
	"ulaundry/internal/telemetry"
)

type actorKey struct{}

func ActorID(ctx context.Context) string { value, _ := ctx.Value(actorKey{}).(string); return value }

type Middleware struct {
	sessions        *security.Manager
	trustedOrigins  map[string]bool
	trackingLimiter *security.Limiter
	logger          *slog.Logger
	metrics         *telemetry.Registry
}

func New(trustedOrigins []string, sessions *security.Manager) *Middleware {
	origins := make(map[string]bool, len(trustedOrigins))
	for _, origin := range trustedOrigins {
		if origin != "" && origin != "*" {
			origins[origin] = true
		}
	}
	return &Middleware{sessions: sessions, trustedOrigins: origins, trackingLimiter: &security.Limiter{}}
}

func (m *Middleware) BukaPintuCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if len(r.Header.Values("Origin")) > 1 || len(r.Header.Values("Origin")) == 1 && origin == "" {
			response.Fail(w, r, http.StatusForbidden, "origin_forbidden", "Origin tidak diizinkan.")
			return
		}
		if origin != "" {
			if !m.trustedOrigins[origin] {
				response.Fail(w, r, http.StatusForbidden, "origin_forbidden", "Origin tidak diizinkan.")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Deprecation, Link, Location")
		}
		if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			method := r.Header.Get("Access-Control-Request-Method")
			if len(r.Header.Values("Access-Control-Request-Method")) != 1 || len(r.Header.Values("Access-Control-Request-Headers")) > 1 || origin == "" || (method != "GET" && method != "HEAD" && method != "POST" && method != "PUT") {
				response.Fail(w, r, http.StatusForbidden, "preflight_forbidden", "Preflight tidak diizinkan.")
				return
			}
			for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				name := strings.ToLower(strings.TrimSpace(header))
				if name != "" && name != "content-type" && name != "authorization" && name != "idempotency-key" && name != "x-csrf-token" {
					response.Fail(w, r, http.StatusForbidden, "preflight_forbidden", "Header preflight tidak diizinkan.")
					return
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-CSRF-Token")
			response.Prepare(w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// TrackingLimit uses the TCP peer only; untrusted forwarding headers cannot
// create new buckets. Tokens and URLs are never used as limiter keys or logged.
func (m *Middleware) TrackingLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			peer = r.RemoteAddr
		}
		if ip := net.ParseIP(peer); ip != nil {
			peer = ip.String()
		}
		allowed, retry, _ := m.trackingLimiter.Take(time.Now(),
			security.Limit{Key: "global", Count: 300, Window: time.Minute},
			security.Limit{Key: "peer:" + peer, Count: 60, Window: time.Minute})
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			response.Fail(w, r, 429, "rate_limited", "Terlalu banyak permintaan. Silakan coba kembali nanti.")
			return
		}
		next(w, r)
	}
}

type claimsKey struct{}

func SessionClaims(ctx context.Context) security.Claims {
	c, _ := ctx.Value(claimsKey{}).(security.Claims)
	return c
}

// ValidasiJWT accepts only server-backed cookie sessions. Bearer is retired.
func (m *Middleware) ValidasiJWT(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := m.sessions.Authenticate(r)
		if err != nil {
			if errors.Is(err, security.ErrUnauthorized) {
				response.Fail(w, r, 401, "unauthorized", "Sesi tidak valid atau telah berakhir.")
			} else {
				response.Fail(w, r, 503, "service_unavailable", "Layanan autentikasi sementara tidak tersedia.")
			}
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !m.sessions.VerifyCSRF(r, c) {
			response.Fail(w, r, 403, "csrf_failed", "Permintaan memerlukan token CSRF dan origin yang sah.")
			return
		}
		ctx := context.WithValue(r.Context(), actorKey{}, c.Subject)
		ctx = context.WithValue(ctx, claimsKey{}, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
