package middleware

import (
	"net/http"
	"ulaundry/internal/http/response"
)

type observedWriter struct {
	http.ResponseWriter
	started bool
}

func (w *observedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *observedWriter) WriteHeader(status int) {
	if !w.started {
		w.started = true
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *observedWriter) Write(body []byte) (int, error) {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// RequestContext is outermost: correlation survives auth/CORS/router failures.
// Panic values/stack traces never become response data or request logs here.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = response.NewRequest(r)
		w.Header().Set("X-Request-ID", response.RequestID(r.Context()))
		tracked := &observedWriter{ResponseWriter: w}
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler || tracked.started {
					panic(http.ErrAbortHandler)
				}
				response.Fail(tracked, r, http.StatusInternalServerError, "internal_error", "Permintaan tidak dapat diproses.")
			}
		}()
		next.ServeHTTP(tracked, r)
	})
}
