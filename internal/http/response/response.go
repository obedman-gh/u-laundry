// Package response contains the shared HTTP representation and request ID.
package response

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strconv"
	"ulaundry/internal/security"
)

type requestKey struct{}
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestKey{}).(string)
	return value
}
func NewRequest(r *http.Request) *http.Request {
	// Mint our own ID: caller headers cannot inject tokens or PII into correlation.
	id := rand.Text()
	return r.WithContext(security.WithCorrelationID(context.WithValue(r.Context(), requestKey{}, id), id))
}
func Prepare(w http.ResponseWriter, r *http.Request) string {
	id := RequestID(r.Context())
	if id == "" {
		id = w.Header().Get("X-Request-ID")
		if id == "" {
			id = rand.Text()
		}
	}
	w.Header().Set("X-Request-ID", id)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return id
}

// JSON encodes before committing headers so marshal failure becomes a safe 500,
// instead of a partial success. Network write failure cannot undo a use case.
func JSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, "internal_error", "Permintaan tidak dapat diproses.")
		return
	}
	Prepare(w, r)
	body = append(body, '\n')
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}
func Fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	id := Prepare(w, r)
	JSON(w, r, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message, RequestID: id}})
}
