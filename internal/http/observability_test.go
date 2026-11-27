package http

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/service"
	"ulaundry/internal/telemetry"
	"ulaundry/internal/testutil"
)

func TestRouterLogLabelsNeverContainClientData(t *testing.T) {
	for _, tc := range []struct {
		method, path, origin, route string
		status                      int
	}{
		{"GET", "/api/tracking/PRIVATEtoken?name=PRIVATEname", "", "/api/tracking/{token}", 204},
		{"GET", "/api/pesanan/PRIVATEorder/struk?phone=PRIVATEphone", "", "/api/pesanan/{id}/struk", 401},
		{"GET", "/api/PRIVATEpath?token=PRIVATEquery", "", "api.unmatched", 404},
		{"GET", "/PRIVATEfile.html", "", "static", 404},
		{"GET", "/api/tracking/PRIVATEtoken", "https://PRIVATEorigin.invalid", "/api/tracking/{token}", 403},
		{"PRIVATE-METHOD", "/api/PRIVATEpath", "", "api.unmatched", 404},
	} {
		t.Run(tc.path+tc.origin, func(t *testing.T) {
			var out bytes.Buffer
			m := telemetry.NewRegistry()
			router := NewRouter(&endpointRecorder{}, middleware.New(nil, testutil.TestSessions("synthetic-router-logs-secret-32bytes")).WithObservability(telemetry.NewLogger(&out, slog.LevelInfo), m), http.FS(fstest.MapFS{}))
			r := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("X-Request-ID", "PRIVATEcaller")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			var event map[string]any
			if err := json.Unmarshal(out.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || event["route"] != tc.route || strings.Contains(out.String(), "PRIVATE") {
				t.Fatal("client data in log", out.String())
			}
			s := m.Snapshot()
			if len(s.Requests) != 1 || s.Requests[0].Route != tc.route {
				t.Fatal("raw metric label")
			}
		})
	}
}
func TestOperationalMetricsOwnerOnlyAndNoSensitiveFields(t *testing.T) {
	manager := testutil.TestSessions("synthetic-operations-http-secret-32bytes")
	token, err := manager.Tokens.Sign(domain.Staff{ID: "actor-1", Username: "staff.test", Role: domain.Owner}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
		work := &roleWork{role: role}
		uow := service.WithAccessAudit(work, work)
		m := telemetry.NewRegistry()
		m.Business(telemetry.OrderCreated)
		h := handler.New(handler.Dependencies{Operations: service.NewOperations(uow), Metrics: m, PoolStats: func() telemetry.Pool { return telemetry.Pool{MaxOpen: 10, InUse: 2} }})
		router := NewRouter(h, middleware.New(nil, manager), http.FS(fstest.MapFS{}))
		for _, method := range []string{"GET", "HEAD"} {
			r := httptest.NewRequest(method, "/api/ops/metrics", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatal("anonymous metrics")
			}
			r.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: token})
			w = httptest.NewRecorder()
			router.ServeHTTP(w, r)
			want := 403
			if role == domain.Owner {
				want = 200
			}
			if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("metrics permission", role, w.Code)
			}
			if role == domain.Owner && method == "GET" {
				for _, key := range []string{`"orders_created":1`, `"max_open":10`, `"in_use":2`} {
					if !strings.Contains(w.Body.String(), key) {
						t.Fatal("missing metric", key)
					}
				}
				for _, private := range []string{"actor-1", "staff.test", token, "customer", "password", "dsn", "SECRET"} {
					if strings.Contains(w.Body.String(), private) {
						t.Fatal("private metrics field", private)
					}
				}
			}
		}
		if role != domain.Owner && len(work.denied) != 2 {
			t.Fatal("denial audit missing")
		}
	}
}
