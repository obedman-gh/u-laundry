package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestMarshalFailureIsSafeError(t *testing.T) {
	r := NewRequest(httptest.NewRequest("GET", "/api/test", nil))
	w := httptest.NewRecorder()
	JSON(w, r, 201, make(chan int))
	var body ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 || body.Error.Code != "internal_error" || body.Error.RequestID != RequestID(r.Context()) {
		t.Fatal("marshal failure committed success")
	}
}
func TestHeadErrorOmitsBody(t *testing.T) {
	w := httptest.NewRecorder()
	r := NewRequest(httptest.NewRequest("HEAD", "/api/test", nil))
	Fail(w, r, 404, "not_found", "Data tidak ditemukan.")
	if w.Code != 404 || w.Body.Len() != 0 || w.Header().Get("Content-Length") == "" {
		t.Fatal("HEAD must retain headers without body")
	}
}
