// Package dto defines external HTTP representations using primitive types.
// It has no domain, repository or use-case dependency.
package dto

import (
	"encoding/base64"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func bad(message string) error           { return &ValidationError{message} }
func Identifier(value string) bool {
	if len(value) < 1 || len(value) > 100 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
func TrackingToken(value string) bool {
	if len(value) != 43 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == value
}
func text(value string, max int, required, multiline bool) bool {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max || required && value == "" {
		return false
	}
	for _, r := range value {
		if multiline && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
func Status(value string) bool {
	return value == "received" || value == "processing" || value == "ready" || value == "collected" || value == "cancelled"
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (v LoginRequest) Validate() error {
	if !text(v.Username, 50, true, false) || v.Password == "" || len(v.Password) > 72 {
		return bad("Username dan kata sandi wajib diisi sesuai batas panjang.")
	}
	return nil
}

type CustomerRequest struct {
	Name    string `json:"name"`
	Contact string `json:"contact"`
	Notes   string `json:"notes"`
}

func (v CustomerRequest) Validate() error {
	if !text(v.Name, 100, true, false) || len(v.Contact) > 64 || !text(v.Notes, 1000, false, true) {
		return bad("Nama, kontak atau catatan pelanggan tidak valid.")
	}
	return nil
}

type ServiceRequest struct {
	Name      string `json:"name"`
	Unit      string `json:"unit"`
	UnitPrice int64  `json:"unit_price"`
	Minutes   int64  `json:"turnaround_minutes"`
	Active    *bool  `json:"active"`
	Version   int64  `json:"version"`
}

func (v ServiceRequest) Validate() error {
	if !text(v.Name, 100, true, false) || (v.Unit != "kg" && v.Unit != "piece") || v.UnitPrice < 0 || v.Minutes <= 0 || v.Minutes > math.MaxInt64/int64(time.Minute) || v.Active == nil || v.Version < 0 {
		return bad("Konfigurasi layanan wajib lengkap dan valid.")
	}
	return nil
}

type ItemRequest struct {
	ServiceID string `json:"service_id"`
	Pieces    int64  `json:"pieces"`
	Grams     int64  `json:"grams"`
}
type OrderRequest struct {
	CustomerID       string        `json:"customer_id"`
	Items            []ItemRequest `json:"items"`
	Discount         int64         `json:"discount"`
	DiscountReason   string        `json:"discount_reason"`
	Notes            string        `json:"notes"`
	EstimatedReadyAt time.Time     `json:"estimated_ready_at"`
}

func (v OrderRequest) Validate() error {
	if !Identifier(v.CustomerID) || len(v.Items) < 1 || len(v.Items) > 100 {
		return bad("Customer dan 1–100 item pesanan wajib diisi.")
	}
	for _, item := range v.Items {
		if !Identifier(item.ServiceID) || !((item.Pieces > 0 && item.Grams == 0) || (item.Grams > 0 && item.Pieces == 0)) {
			return bad("Layanan dan jumlah atau gram positif wajib diisi per item.")
		}
	}
	if v.Discount < 0 || !text(v.DiscountReason, 250, v.Discount > 0, false) || v.Discount == 0 && strings.TrimSpace(v.DiscountReason) != "" || !text(v.Notes, 1000, false, true) {
		return bad("Diskon, alasan diskon atau catatan tidak valid.")
	}
	return nil
}

type StatusRequest struct {
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

func (v StatusRequest) Validate() error {
	if !Status(v.Status) || v.Status == "cancelled" || v.Version < 1 {
		return bad("Status progres dan versi positif wajib diisi; pembatalan memakai endpoint cancel.")
	}
	return nil
}

type CancelRequest struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

func (v CancelRequest) Validate() error {
	if v.Version < 1 || !text(v.Reason, 250, true, false) {
		return bad("Versi positif dan alasan pembatalan wajib diisi.")
	}
	return nil
}

type PaymentRequest struct {
	Amount int64  `json:"amount"`
	Method string `json:"method"`
}

func (v PaymentRequest) Validate() error {
	if v.Amount <= 0 || (v.Method != "cash" && v.Method != "bank_transfer" && v.Method != "qris") {
		return bad("Nominal positif dan metode pembayaran yang sah wajib diisi.")
	}
	return nil
}
