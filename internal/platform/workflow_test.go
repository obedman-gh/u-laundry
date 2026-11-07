package platform

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"ulaundry/internal/domain"
)

func TestWorkflowGeneratorsAndJWTIdentity(t *testing.T) {
	id, err := (RandomIDs{}).New()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != 16 {
		t.Fatal("ID format")
	}
	token, err := (RandomTokens{}).New()
	if err != nil {
		t.Fatal(err)
	}
	raw, err = base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		t.Fatal("tracking entropy size")
	}
	const secret = "synthetic-platform-jwt-secret-32bytes"
	at := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
	signed, err := (JWTIssuer{Secret: secret}).Sign(domain.Staff{ID: "staff-1", Username: "staff.test", Role: domain.Cashier}, at)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.Parse(signed, func(t *jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(func() time.Time { return at }))
	if err != nil || !parsed.Valid {
		t.Fatal(err)
	}
	subject, err := parsed.Claims.GetSubject()
	if err != nil || subject != "staff-1" {
		t.Fatal("identity missing")
	}
	issued, err := parsed.Claims.GetIssuedAt()
	if err != nil || !issued.Time.Equal(at) {
		t.Fatal("clock ignored")
	}
	if _, err := (JWTIssuer{}).Sign(domain.Staff{ID: "staff-1"}, at); err == nil {
		t.Fatal("unsafe signing key accepted")
	}
}
