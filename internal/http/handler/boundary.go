package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ulaundry/internal/domain"
	"ulaundry/internal/http/dto"
	"ulaundry/internal/http/response"
	"ulaundry/internal/repository"
	"ulaundry/internal/security"
	"ulaundry/internal/service"
)

const MaxBodyBytes int64 = 64 << 10

type validatedRequest interface{ Validate() error }

func decode(w http.ResponseWriter, r *http.Request, value validatedRequest) bool {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || len(r.Header.Values("Content-Type")) != 1 || media != "application/json" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		response.Fail(w, r, 415, "unsupported_media_type", "Gunakan Content-Type application/json dengan UTF-8.")
		return false
	}
	if encoding := r.Header.Get("Content-Encoding"); len(r.Header.Values("Content-Encoding")) > 1 || encoding != "" && !strings.EqualFold(encoding, "identity") {
		response.Fail(w, r, 415, "unsupported_content_encoding", "Body terkompresi tidak didukung.")
		return false
	}
	if r.ContentLength > MaxBodyBytes {
		response.Fail(w, r, 413, "payload_too_large", "Body maksimal 64 KiB.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			response.Fail(w, r, 413, "payload_too_large", "Body maksimal 64 KiB.")
		} else {
			response.Fail(w, r, 400, "invalid_body", "Body permintaan tidak dapat dibaca.")
		}
		return false
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' || !utf8.Valid(body) {
		response.Fail(w, r, 400, "invalid_json", "Kirim satu objek JSON UTF-8 yang valid.")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		code, message := "invalid_json", "Format atau tipe data JSON tidak valid."
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			code, message = "unknown_field", "Body mengandung field yang tidak didukung."
		}
		response.Fail(w, r, 400, code, message)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		response.Fail(w, r, 400, "invalid_json", "Kirim hanya satu objek JSON.")
		return false
	}
	// encoding/json accepts repeated fields with last-value-wins semantics.
	// Reject ambiguous keys (including case aliases) before calling a use case.
	keys := json.NewDecoder(bytes.NewReader(body))
	keys.UseNumber()
	if err := uniqueKeys(keys); err != nil {
		response.Fail(w, r, 400, "invalid_json", "Objek JSON tidak boleh mengulang field.")
		return false
	}
	if err := value.Validate(); err != nil {
		writeError(w, r, err)
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := 500, "internal_error", "Permintaan tidak dapat diproses."
	var business *domain.Error
	var boundary *dto.ValidationError
	switch {
	case errors.Is(err, repository.ErrInvalidData), errors.Is(err, repository.ErrConstraint):
	case errors.Is(err, repository.ErrUnavailable):
		status, code, message = 503, "service_unavailable", "Layanan penyimpanan sementara tidak tersedia."
	case errors.Is(err, service.ErrAuthentication), errors.Is(err, security.ErrUnauthorized):
		status, code, message = 401, "unauthorized", "Username atau kata sandi tidak valid."
	case errors.Is(err, context.DeadlineExceeded):
		status, code, message = 504, "deadline_exceeded", "Waktu permintaan telah habis."
	case errors.Is(err, context.Canceled):
		status, code, message = 408, "request_canceled", "Permintaan telah dibatalkan."
	case errors.As(err, &boundary):
		status, code, message = 400, "validation_failed", boundary.Message
	case errors.As(err, &business):
		// Raw domain/repository messages are never serialized. This also protects
		// against future adapters accidentally wrapping SQL/credential details.
		if business.Field == "clock" {
			break
		}
		switch business.Kind {
		case domain.Validation:
			status, code, message = 400, "validation_failed", "Data tidak memenuhi ketentuan."
		case domain.Forbidden:
			status, code, message = 403, "forbidden", "Staf tidak diizinkan menjalankan tindakan ini."
		case domain.NotFound:
			status, code, message = 404, "not_found", "Data tidak ditemukan."
		case domain.Conflict:
			status, code, message = 409, "conflict", "Data telah berubah atau tindakan bertentangan dengan keadaan saat ini."
		}
	case errors.Is(err, repository.ErrNotFound):
		status, code, message = 404, "not_found", "Data tidak ditemukan."
	case errors.Is(err, repository.ErrConflict):
		status, code, message = 409, "conflict", "Data telah berubah atau kunci telah digunakan."
	}
	response.Fail(w, r, status, code, message)
}

func query(r *http.Request, allowed ...string) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, &dto.ValidationError{Message: "Format query tidak valid."}
	}
	keys := map[string]bool{}
	for _, key := range allowed {
		keys[key] = true
	}
	for key, entries := range values {
		if !keys[key] || len(entries) != 1 || entries[0] == "" {
			return nil, &dto.ValidationError{Message: "Parameter query tidak didukung, kosong atau berulang."}
		}
	}
	return values, nil
}
func page(values url.Values) (repository.Page, error) {
	p := repository.Page{Limit: 20}
	for _, field := range []struct {
		key    string
		target *int
	}{{"limit", &p.Limit}, {"offset", &p.Offset}} {
		if raw := values.Get(field.key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return p, &dto.ValidationError{Message: "Pagination harus bilangan bulat dalam batas yang didukung."}
			}
			*field.target = n
		}
	}
	if p.Limit < 1 || p.Limit > 100 || p.Offset < 0 {
		return p, &dto.ValidationError{Message: "Limit harus 1–100 dan offset minimal 0."}
	}
	return p, nil
}
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !dto.Identifier(id) {
		response.Fail(w, r, 400, "invalid_path_parameter", "Identitas path tidak valid.")
		return "", false
	}
	return id, true
}
func nullableTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
}

// uniqueKeys walks one already-decoded JSON value, including nested objects.
func uniqueKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok {
				return errors.New("invalid object key")
			}
			// Match encoding/json's Unicode case folding as well as ASCII aliases.
			for previous := range seen {
				if strings.EqualFold(previous, key) {
					return errors.New("duplicate object key")
				}
			}
			seen[key] = true
			if err := uniqueKeys(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueKeys(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid delimiter")
	}
	_, err = decoder.Token()
	return err
}
