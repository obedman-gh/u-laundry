package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ulaundry/internal/telemetry"
)

func TestRequestLogRecoveryClassificationAndRedaction(t *testing.T) {
	for _, name := range []string{"success", "denial", "panic", "partial", "flush", "abort", "implicit"} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			metrics := telemetry.NewRegistry()
			m := New(nil, nil).WithObservability(telemetry.NewLogger(&out, slog.LevelDebug), metrics)
			h := m.ObserveRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				telemetry.SetRoute(r.Context(), "/api/tracking/{token}")
				switch name {
				case "success":
					w.WriteHeader(204)
				case "denial":
					w.WriteHeader(401)
				case "panic":
					w.Header().Set("Content-Length", "999")
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set("Set-Cookie", "private-cookie")
					w.Header().Set("Location", "/private-customer")
					panic("private-driver-token")
				case "partial":
					w.WriteHeader(201)
					panic("private-driver-token")
				case "flush":
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Fatal(err)
					}
					panic("private-driver-token")
				case "abort":
					panic(http.ErrAbortHandler)
				case "implicit":
					_, _ = w.Write([]byte("ok"))
				}
			}))
			r := httptest.NewRequest("GET", "/api/tracking/private-token?name=private-customer", strings.NewReader("private-password"))
			r.Header.Set("Authorization", "Bearer private-jwt")
			r.Header.Set("Cookie", "private-cookie")
			r.Header.Set("X-Request-ID", "private-caller-id")
			r.Header.Set("User-Agent", "private-agent")
			w := httptest.NewRecorder()
			func() {
				defer func() {
					v := recover()
					if name == "partial" || name == "flush" || name == "abort" {
						if v != http.ErrAbortHandler {
							t.Errorf("unsafe abort %v", v)
						}
					} else if v != nil {
						t.Errorf("unrecovered panic %v", v)
					}
				}()
				h.ServeHTTP(w, r)
			}()
			var event map[string]any
			if err := json.Unmarshal(out.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			wantClass, wantStatus, wantLevel := "none", 204.0, "INFO"
			switch name {
			case "denial":
				wantClass, wantStatus, wantLevel = "authentication", 401, "WARN"
			case "panic":
				wantClass, wantStatus, wantLevel = "panic", 500, "ERROR"
			case "partial":
				wantClass, wantStatus, wantLevel = "panic", 201, "ERROR"
			case "flush":
				wantClass, wantStatus, wantLevel = "panic", 200, "ERROR"
			case "abort":
				wantClass, wantStatus, wantLevel = "connection_aborted", 0, "ERROR"
			case "implicit":
				wantStatus = 200
			}
			if event["route"] != "/api/tracking/{token}" || event["error_class"] != wantClass || event["status"] != wantStatus || event["level"] != wantLevel || event["request_id"] != w.Header().Get("X-Request-ID") || event["duration_ms"].(float64) < 0 || strings.Contains(out.String(), "private-") {
				t.Fatalf("unsafe log %s", out.String())
			}
			if name == "panic" {
				if w.Code != 500 || w.Header().Get("Content-Encoding") != "" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), "private-") {
					t.Fatal("panic response leak")
				}
			}
			s := metrics.Snapshot()
			if len(s.Requests) != 1 || s.Requests[0].Count != 1 || (s.Requests[0].Errors == 1) != (wantClass != "none") {
				t.Fatal("request metrics diverged")
			}
		})
	}
}

type brokenWriter struct{ header http.Header }

func (w brokenWriter) Header() http.Header       { return w.header }
func (w brokenWriter) WriteHeader(int)           {}
func (w brokenWriter) Write([]byte) (int, error) { return 0, errors.New("private-network-message") }
func TestResponseWriteFailureIsClassifiedWithoutRawError(t *testing.T) {
	var out bytes.Buffer
	m := New(nil, nil).WithObservability(telemetry.NewLogger(&out, slog.LevelInfo), telemetry.NewRegistry())
	h := m.ObserveRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	h.ServeHTTP(brokenWriter{make(http.Header)}, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(out.String(), `"error_class":"response_write"`) || strings.Contains(out.String(), "private-") {
		t.Fatal("write failure leaked", out.String())
	}
}

func TestPanicRecoveryKeepsLocalServerAvailable(t *testing.T) {
	var out bytes.Buffer
	metrics := telemetry.NewRegistry()
	logger := telemetry.NewLogger(&out, slog.LevelInfo)
	m := New(nil, nil).WithObservability(logger, metrics)
	h := m.ObserveRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		telemetry.SetRoute(r.Context(), "fixture.health")
		if r.URL.Path == "/fail" {
			panic("private-panic-connection-value")
		}
		w.WriteHeader(204)
	}))
	ts := httptest.NewUnstartedServer(h)
	ts.Config.ErrorLog = log.New(telemetry.ServerErrorWriter{Logger: logger}, "", 0)
	ts.Start()
	defer ts.Close()
	for _, tc := range []struct {
		path   string
		status int
	}{{"/fail", 500}, {"/live", 204}} {
		res, err := ts.Client().Get(ts.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		closeErr := res.Body.Close()
		if err != nil || closeErr != nil || res.StatusCode != tc.status || strings.Contains(string(body), "private-") {
			t.Fatal("recovery failed", res.StatusCode, err, closeErr)
		}
	}
	if strings.Contains(out.String(), "private-") {
		t.Fatal("server leaked panic")
	}
	var count, failed uint64
	for _, v := range metrics.Snapshot().Requests {
		count += v.Count
		failed += v.Errors
	}
	if count != 2 || failed != 1 {
		t.Fatal("recovery metrics", count, failed)
	}
}
