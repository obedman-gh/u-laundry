package domain_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
	"ulaundry/internal/domain"
)

func TestTrackingTokenPolicy(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32))
	hash, ok := domain.TrackingHash(token)
	if !ok || hash != sha256.Sum256([]byte(token)) || len(domain.TrackingCode(hash)) != 12 || domain.TrackingLifetime != 90*24*time.Hour {
		t.Fatal("capability policy")
	}
	for _, invalid := range []string{"", "ORDER-123", token + "=", token[:42] + "p", strings.Repeat("A", 42), strings.Repeat("A", 44), strings.Repeat("!", 43)} {
		if _, ok := domain.TrackingHash(invalid); ok {
			t.Fatalf("accepted malformed token %q", invalid)
		}
	}
	for _, status := range []domain.OrderStatus{domain.Received, domain.Processing, domain.Ready, domain.Collected, domain.Cancelled} {
		if domain.TrackingMessage(status) == "" {
			t.Fatal("missing public message")
		}
	}
}

func TestCustomerMinimumDataAndEdit(t *testing.T) {
	c, err := domain.NewCustomer("customer-1", " Nama ", "", "", now)
	if err != nil || c.Contact != "" || c.Notes != "" || c.Address != "" {
		t.Fatal("name-only customer", err)
	}
	updated, err := c.Edit(" Nama Baru ", "0812-3456-7890", " Alamat\r\nBaris 2 ", " Catatan ", now.Add(time.Hour))
	if err != nil || updated.Name != "Nama Baru" || updated.Contact != "+6281234567890" || updated.Address != "Alamat\nBaris 2" || updated.Notes != "Catatan" || !updated.CreatedAt.Equal(c.CreatedAt) {
		t.Fatal(updated, err)
	}
	cleared, err := updated.Edit("Nama Baru", "", "", "", now.Add(2*time.Hour))
	if err != nil || cleared.Contact != "" || cleared.Address != "" || cleared.Notes != "" {
		t.Fatal("optional data cannot be cleared", err)
	}
	for _, address := range []string{strings.Repeat("x", 501), "Alamat\x00"} {
		if _, err := c.Edit(c.Name, "", address, "", now); err == nil {
			t.Fatal("invalid address accepted")
		}
	}
	for _, term := range []string{strings.Repeat("x", 101), "Nama\nBaru", string([]byte{255})} {
		if _, err := domain.CustomerSearch(term); err == nil {
			t.Fatal("invalid search accepted")
		}
	}
	if q, err := domain.CustomerSearch(" 0812-3456-7890 "); err != nil || q != "+6281234567890" {
		t.Fatal("contact search normalization")
	}
}
