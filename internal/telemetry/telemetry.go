// Package telemetry stores bounded, non-sensitive process diagnostics. It is
// separate from financial records and the persistent business/security audit.
package telemetry

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"
)

func NewLogger(out io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))
}

// net/http diagnostics can contain peer addresses, URLs and panic values. Do
// not forward arbitrary stdlib text into the structured application log.
type ServerErrorWriter struct{ Logger *slog.Logger }

func (w ServerErrorWriter) Write(p []byte) (int, error) {
	w.Logger.Error("http.server_error", "error_class", "transport")
	return len(p), nil
}

type requestKey struct{}
type Request struct{ Route, ErrorClass string }

func NewRequest(ctx context.Context) (context.Context, *Request) {
	r := &Request{Route: "unmatched"}
	return context.WithValue(ctx, requestKey{}, r), r
}
func SetRoute(ctx context.Context, route string) {
	if r, ok := ctx.Value(requestKey{}).(*Request); ok {
		r.Route = route
	}
}

// Callers supply controlled classifications only, never err.Error().
func SetError(ctx context.Context, class string) {
	if r, ok := ctx.Value(requestKey{}).(*Request); ok {
		r.ErrorClass = class
	}
}
func Method(method string) string {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		return method
	default:
		return "OTHER"
	}
}
func StatusClass(status int) string {
	switch {
	case status < 400:
		return "none"
	case status == 401:
		return "authentication"
	case status == 403:
		return "authorization"
	case status == 404:
		return "not_found"
	case status == 405:
		return "method_not_allowed"
	case status == 408 || status == 504:
		return "timeout"
	case status == 409:
		return "conflict"
	case status == 429:
		return "rate_limited"
	case status == 503:
		return "dependency_unavailable"
	case status < 500:
		return "validation"
	default:
		return "internal"
	}
}

type RequestMetric struct {
	Route         string  `json:"route"`
	Method        string  `json:"method"`
	Status        int     `json:"status"`
	Count         uint64  `json:"count"`
	Errors        uint64  `json:"errors"`
	DurationMS    float64 `json:"duration_ms_total"`
	MaxDurationMS float64 `json:"duration_ms_max"`
}
type Business struct {
	OrdersCreated    uint64 `json:"orders_created"`
	PaymentsRecorded uint64 `json:"payments_recorded"`
	RefundsRecorded  uint64 `json:"refunds_recorded"`
}
type BusinessEvent uint8

const (
	OrderCreated BusinessEvent = iota
	PaymentRecorded
	RefundRecorded
)

type key struct {
	route, method string
	status        int
}
type Registry struct {
	mu       sync.Mutex
	requests map[key]RequestMetric
	business Business
	started  time.Time
}

func NewRegistry() *Registry {
	return &Registry{requests: make(map[key]RequestMetric), started: time.Now().UTC()}
}
func (m *Registry) Observe(route, method string, status int, elapsed time.Duration, failed bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key{route, Method(method), status}
	// Routes are fixed server patterns; the additional cap bounds future mistakes.
	if _, exists := m.requests[k]; !exists && len(m.requests) >= 511 {
		k = key{"overflow", "OTHER", 0}
	}
	v := m.requests[k]
	v.Route, v.Method, v.Status = k.route, k.method, k.status
	v.Count++
	if failed {
		v.Errors++
	}
	ms := float64(elapsed) / float64(time.Millisecond)
	v.DurationMS += ms
	if ms > v.MaxDurationMS {
		v.MaxDurationMS = ms
	}
	m.requests[k] = v
}
func (m *Registry) Business(event BusinessEvent) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch event {
	case OrderCreated:
		m.business.OrdersCreated++
	case PaymentRecorded:
		m.business.PaymentsRecorded++
	case RefundRecorded:
		m.business.RefundsRecorded++
	}
}

type Snapshot struct {
	StartedAt time.Time       `json:"started_at"`
	Requests  []RequestMetric `json:"requests"`
	Business  Business        `json:"business"`
}

func (m *Registry) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Snapshot{StartedAt: m.started, Requests: make([]RequestMetric, 0, len(m.requests)), Business: m.business}
	for _, v := range m.requests {
		s.Requests = append(s.Requests, v)
	}
	sort.Slice(s.Requests, func(i, j int) bool {
		a, b := s.Requests[i], s.Requests[j]
		if a.Route != b.Route {
			return a.Route < b.Route
		}
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		return a.Status < b.Status
	})
	return s
}

type Pool struct {
	MaxOpen           int     `json:"max_open"`
	Open              int     `json:"open"`
	InUse             int     `json:"in_use"`
	Idle              int     `json:"idle"`
	WaitCount         int64   `json:"wait_count"`
	WaitDurationMS    float64 `json:"wait_duration_ms"`
	MaxIdleClosed     int64   `json:"max_idle_closed"`
	MaxIdleTimeClosed int64   `json:"max_idle_time_closed"`
	MaxLifetimeClosed int64   `json:"max_lifetime_closed"`
}

func DBPool(s sql.DBStats) Pool {
	return Pool{s.MaxOpenConnections, s.OpenConnections, s.InUse, s.Idle, s.WaitCount, float64(s.WaitDuration) / float64(time.Millisecond), s.MaxIdleClosed, s.MaxIdleTimeClosed, s.MaxLifetimeClosed}
}
