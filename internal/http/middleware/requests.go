package middleware

import (
	"log/slog"
	"net/http"
	"time"
	"ulaundry/internal/http/response"
	"ulaundry/internal/telemetry"
)

type observedWriter struct {
	http.ResponseWriter
	started     bool
	status      int
	writeFailed bool
}

func (w *observedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *observedWriter) FlushError() error {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err != nil {
		w.writeFailed = true
	}
	return err
}
func (w *observedWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 && status != 101 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if !w.started {
		w.started = true
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *observedWriter) Write(body []byte) (int, error) {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	if err != nil {
		w.writeFailed = true
	}
	return n, err
}

// RequestContext is outermost: correlation survives auth/CORS/router failures.
// Panic values/stack traces never become response data or request logs here.
func RequestContext(next http.Handler) http.Handler {
	return requestContext(next, nil, nil)
}

func (m *Middleware) WithObservability(logger *slog.Logger, metrics *telemetry.Registry) *Middleware {
	m.logger, m.metrics = logger, metrics
	return m
}
func (m *Middleware) ObserveRequests(next http.Handler) http.Handler {
	return requestContext(next, m.logger, m.metrics)
}

func requestContext(next http.Handler, logger *slog.Logger, metrics *telemetry.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		r = response.NewRequest(r)
		ctx, observation := telemetry.NewRequest(r.Context())
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", response.RequestID(r.Context()))
		tracked := &observedWriter{ResponseWriter: w}
		defer func() {
			abort := false
			if value := recover(); value != nil {
				observation.ErrorClass = "panic"
				if value == http.ErrAbortHandler || tracked.started {
					abort = true
					if value == http.ErrAbortHandler {
						observation.ErrorClass = "connection_aborted"
					}
				} else {
					// Discard representation/session headers from interrupted work.
					for _, name := range []string{"Content-Length", "Content-Type", "Content-Encoding", "Content-Disposition", "ETag", "Last-Modified", "Location", "Set-Cookie"} {
						w.Header().Del(name)
					}
					response.Fail(tracked, r, http.StatusInternalServerError, "internal_error", "Permintaan tidak dapat diproses.")
				}
			}
			status := tracked.status
			if status == 0 && !abort {
				status = 200
			}
			class := observation.ErrorClass
			if class == "" {
				class = telemetry.StatusClass(status)
			}
			if tracked.writeFailed {
				class = "response_write"
			}
			elapsed := time.Since(start)
			metrics.Observe(observation.Route, r.Method, status, elapsed, class != "none")
			if logger != nil {
				level := slog.LevelInfo
				if status >= 400 {
					level = slog.LevelWarn
				}
				if status >= 500 || class == "panic" || class == "response_write" || abort {
					level = slog.LevelError
				}
				logger.Log(r.Context(), level, "http.request", "request_id", response.RequestID(r.Context()), "route", observation.Route, "method", telemetry.Method(r.Method), "status", status, "duration_ms", float64(elapsed)/float64(time.Millisecond), "error_class", class, "response_aborted", abort)
			}
			if abort {
				panic(http.ErrAbortHandler)
			}
		}()
		next.ServeHTTP(tracked, r)
	})
}
