package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"ulaundry/internal/http/response"
)

const MaxBodyBytes int64 = 64 << 10

// SecurityHeaders also covers static pages, redirects, denials and errors.
// HSTS follows validated deployment configuration, never forwarding headers.
func (m *Middleware) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; manifest-src 'self'; worker-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		if m.sessions != nil && m.sessions.Secure && strings.HasPrefix(m.sessions.Origin, "https://") {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// RequestLimits checks every API route, even bodyless commands and unknown
// paths. Bounded reads cover chunked bodies before authentication/database work.
func RequestLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			if r.ContentLength > MaxBodyBytes {
				response.Fail(w, r, 413, "payload_too_large", "Body maksimal 64 KiB.")
				return
			}
			if r.Body != nil {
				limited := http.MaxBytesReader(w, r.Body, MaxBodyBytes)
				defer limited.Close()
				body, err := io.ReadAll(limited)
				if err != nil {
					var oversized *http.MaxBytesError
					if errors.As(err, &oversized) {
						response.Fail(w, r, 413, "payload_too_large", "Body maksimal 64 KiB.")
					} else {
						response.Fail(w, r, 400, "invalid_body", "Body permintaan tidak dapat dibaca.")
					}
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
		}
		next.ServeHTTP(w, r)
	})
}
