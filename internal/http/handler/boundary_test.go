package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/testutil"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

type fakeLogin func(context.Context, string, string) (service.LoginResult, error)

func (f fakeLogin) Login(c context.Context, u, p string) (service.LoginResult, error) {
	return f(c, u, p)
}
func checkError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body response.ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("non-JSON response: %v", err)
	}
	if w.Code != status || body.Error.Code != code || body.Error.Message == "" || body.Error.RequestID == "" || body.Error.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatalf("unexpected error contract: %d %+v", w.Code, body)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatal("missing JSON/privacy headers")
	}
}
func TestJSONRequestBoundary(t *testing.T) {
	const valid = `{"username":"staff.test","password":"synthetic"}`
	for _, tc := range []struct {
		name, body, media, encoding                string
		chunked, duplicateMedia, duplicateEncoding bool
		status                                     int
		code                                       string
	}{
		{name: "success", body: valid, media: "application/json", status: 200},
		{name: "utf8", body: valid, media: "application/json; charset=UTF-8", status: 200},
		{name: "empty", media: "application/json", status: 400, code: "invalid_json"},
		{name: "malformed", body: `{"username":`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "null", body: `null`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "array", body: `[]`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "trailing-object", body: valid + ` {}`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "trailing-garbage", body: valid + ` invalid`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "invalid-utf8", body: "{\"username\":\"\xff\",\"password\":\"x\"}", media: "application/json", status: 400, code: "invalid_json"},
		{name: "wrong-type", body: `{"username":42,"password":"x"}`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "unknown", body: `{"username":"staff","password":"x","role":"owner"}`, media: "application/json", status: 400, code: "unknown_field"},
		{name: "duplicate", body: `{"username":"one","username":"two","password":"x"}`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "case-alias", body: `{"username":"one","Username":"two","password":"x"}`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "unicode-case-alias", body: `{"username":"one","u\u017fername":"two","password":"x"}`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "escaped-key", body: `{"username":"one","user\u006eame":"two","password":"x"}`, media: "application/json", status: 400, code: "invalid_json"},
		{name: "required", body: `{}`, media: "application/json", status: 400, code: "validation_failed"},
		{name: "long-password", body: `{"username":"staff","password":"` + strings.Repeat("a", 73) + `"}`, media: "application/json", status: 400, code: "validation_failed"},
		{name: "missing-media", body: valid, status: 415, code: "unsupported_media_type"},
		{name: "text", body: valid, media: "text/plain", status: 415, code: "unsupported_media_type"},
		{name: "bad-charset", body: valid, media: "application/json; charset=latin1", status: 415, code: "unsupported_media_type"},
		{name: "duplicate-media", body: valid, media: "application/json", duplicateMedia: true, status: 415, code: "unsupported_media_type"},
		{name: "compressed", body: valid, media: "application/json", encoding: "gzip", status: 415, code: "unsupported_content_encoding"},
		{name: "duplicate-encoding", body: valid, media: "application/json", encoding: "identity", duplicateEncoding: true, status: 415, code: "unsupported_content_encoding"},
		{name: "known-length-limit", body: strings.Repeat(" ", int(MaxBodyBytes)) + valid, media: "application/json", status: 413, code: "payload_too_large"},
		{name: "stream-limit", body: strings.Repeat(" ", int(MaxBodyBytes)) + valid, media: "application/json", chunked: true, status: 413, code: "payload_too_large"},
		{name: "exact-limit", body: valid + strings.Repeat(" ", int(MaxBodyBytes)-len(valid)), media: "application/json", chunked: true, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			manager := testutil.TestSessions("synthetic-boundary-jwt-secret-32bytes")
			h := New(Dependencies{Sessions: manager, Auth: fakeLogin(func(_ context.Context, u, p string) (service.LoginResult, error) {
				calls++
				if u != "staff.test" || p != "synthetic" {
					t.Fatal("unexpected credentials")
				}
				signed, err := manager.Tokens.Sign(domain.Staff{ID: "staff-1", Username: "staff.test", Role: domain.Cashier}, time.Now())
				return service.LoginResult{Token: signed, Name: "Staf"}, err
			})})
			r := httptest.NewRequest("POST", "/api/login", strings.NewReader(tc.body))
			r.Header.Set("Origin", manager.Origin)
			r.Header.Set("Content-Type", tc.media)
			if tc.duplicateMedia {
				r.Header.Add("Content-Type", "text/plain")
			}
			if tc.encoding != "" {
				r.Header.Set("Content-Encoding", tc.encoding)
			}
			if tc.duplicateEncoding {
				r.Header.Add("Content-Encoding", "gzip")
			}
			if tc.chunked {
				r.ContentLength = -1
			}
			w := httptest.NewRecorder()
			middleware.RequestContext(http.HandlerFunc(h.Login)).ServeHTTP(w, r)
			if tc.status >= 400 {
				checkError(t, w, tc.status, tc.code)
				if calls != 0 {
					t.Fatal("invalid body reached use case")
				}
			} else if w.Code != tc.status || calls != 1 {
				t.Fatalf("status=%d calls=%d", w.Code, calls)
			}
		})
	}
}

type unreadableBody struct{}

func (unreadableBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (unreadableBody) Close() error             { return nil }
func TestUnreadableBody(t *testing.T) {
	h := New(Dependencies{Sessions: testutil.TestSessions("synthetic-boundary-jwt-secret-32bytes")})
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.Header.Set("Origin", "https://fixture.invalid")
	r.Body = unreadableBody{}
	r.ContentLength = -1
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r)
	checkError(t, w, 400, "invalid_body")
}
func TestErrorMappingRedactsInternalDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"authentication", service.ErrAuthentication, 401, "unauthorized"},
		{"domain-validation", &domain.Error{Kind: domain.Validation, Message: "private-secret"}, 400, "validation_failed"},
		{"domain-forbidden", &domain.Error{Kind: domain.Forbidden, Message: "private-secret"}, 403, "forbidden"},
		{"domain-not-found", &domain.Error{Kind: domain.NotFound, Message: "private-secret"}, 404, "not_found"},
		{"domain-conflict", &domain.Error{Kind: domain.Conflict, Message: "private-secret"}, 409, "conflict"},
		{"clock-invalid", &domain.Error{Kind: domain.Validation, Field: "clock", Message: "private-secret"}, 500, "internal_error"},
		{"repository-not-found", repository.ErrNotFound, 404, "not_found"},
		{"repository-conflict", repository.ErrConflict, 409, "conflict"},
		{"repository-unavailable", repository.ErrUnavailable, 503, "service_unavailable"},
		{"stored-invalid", repository.ErrInvalidData, 500, "internal_error"},
		{"constraint", repository.ErrConstraint, 500, "internal_error"},
		{"deadline", context.DeadlineExceeded, 504, "deadline_exceeded"},
		{"canceled", context.Canceled, 408, "request_canceled"},
		{"unexpected", errors.New("private-secret"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/pesanan/one", nil)
			writeError(w, r, fmt.Errorf("private-secret: %w", tc.err))
			checkError(t, w, tc.status, tc.code)
			if strings.Contains(w.Body.String(), "private-secret") {
				t.Fatal("internal details reflected")
			}
		})
	}
}
func TestListRejectsAmbiguousOrUnboundedQueries(t *testing.T) {
	for _, raw := range []string{"limit=0", "limit=101", "limit=-1", "offset=-1", "limit=1.5", "offset=9999999999999999999999999", "limit=1&limit=2", "limit=", "offset=", "unknown=1", "status=unknown", "status=", "customer_id=../private", "limit=%zz", "limit=10;offset=0"} {
		t.Run(raw, func(t *testing.T) {
			h := New(Dependencies{Orders: fakeOrders{list: func(context.Context, string, repository.OrderFilter) ([]service.OrderListItem, error) {
				t.Fatal("invalid query reached use case")
				return nil, nil
			}}})
			w := protected(t, "GET", "/api/pesanan?"+raw, "", h.GetSemuaPesanan)
			checkError(t, w, 400, "validation_failed")
		})
	}
}
func TestOrderNestedDuplicatesAndPath(t *testing.T) {
	h := New(Dependencies{})
	w := protected(t, "POST", "/api/pesanan", `{"customer_id":"c-1","items":[{"service_id":"s-1","grams":1,"grams":2}]}`, h.BikinPesananBaru)
	checkError(t, w, 400, "invalid_json")
	r := httptest.NewRequest("GET", "/api/pesanan/invalid", nil)
	r.SetPathValue("id", "../private")
	w = httptest.NewRecorder()
	h.GetPesananById(w, r)
	checkError(t, w, 400, "invalid_path_parameter")
	r = httptest.NewRequest("GET", "/api/tracking/invalid", nil)
	r.SetPathValue("token", "invalid")
	w = httptest.NewRecorder()
	h.Tracking(w, r)
	checkError(t, w, 404, "not_found")
}

type fakePayment func(context.Context, string, service.PaymentInput) (repository.PaymentResult, error)

func (f fakePayment) Record(c context.Context, a string, in service.PaymentInput) (repository.PaymentResult, error) {
	return f(c, a, in)
}
func TestPaymentHTTPIdempotencyContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keys   []string
		replay bool
		status int
	}{
		{"missing", nil, false, 400}, {"invalid", []string{"../key"}, false, 400}, {"multiple", []string{"key-1", "key-2"}, false, 400},
		{"new", []string{"key-1"}, false, 201}, {"replay", []string{"key-1"}, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := New(Dependencies{Payments: fakePayment(func(_ context.Context, _ string, in service.PaymentInput) (repository.PaymentResult, error) {
				calls++
				if in.IdempotencyKey != "key-1" || in.OrderID != "order-1" || in.Amount != 1000 {
					t.Fatal("payment input changed")
				}
				return repository.PaymentResult{Payment: domain.Payment{ID: "payment-1", Amount: 1000}, Replayed: tc.replay}, nil
			})})
			r := httptest.NewRequest("POST", "/api/pesanan/order-1/pembayaran", strings.NewReader(`{"amount":1000,"method":"cash"}`))
			r.SetPathValue("id", "order-1")
			r.Header.Set("Content-Type", "application/json")
			for _, key := range tc.keys {
				r.Header.Add("Idempotency-Key", key)
			}
			w := httptest.NewRecorder()
			h.RecordPayment(w, r)
			if tc.status == 400 {
				checkError(t, w, 400, "validation_failed")
				if calls != 0 {
					t.Fatal("invalid key reached use case")
				}
			} else {
				if w.Code != tc.status || calls != 1 || !strings.Contains(w.Body.String(), `"payment_id":"payment-1"`) {
					t.Fatalf("unexpected payment result: %d", w.Code)
				}
			}
		})
	}
}
