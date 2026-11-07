package platform

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"ulaundry/internal/domain"
	"ulaundry/internal/security"
)

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type RandomIDs struct{}

func (RandomIDs) New() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", errors.New("Generator ID tidak tersedia")
	}
	return hex.EncodeToString(raw), nil
}

type RandomTokens struct{}

func (RandomTokens) New() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", errors.New("Generator token tidak tersedia")
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

type Passwords struct{}

func (Passwords) Compare(hash, password string) bool {
	if hash == "" {
		hash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// Hash is the entry point for future create/reset commands; existing login
// credentials are not silently changed or subjected to new-password rules.
func (Passwords) Hash(password string) (string, error) {
	if err := domain.ValidateNewPassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.New("Kata sandi tidak dapat disimpan")
	}
	return string(hash), nil
}

type JWTIssuer struct {
	Secret, Issuer, Audience string
	Lifetime                 time.Duration
}

func (s JWTIssuer) Sign(staff domain.Staff, at time.Time) (string, error) {
	issuer, audience, lifetime := s.Issuer, s.Audience, s.Lifetime
	if issuer == "" {
		issuer = "ulaundry"
	}
	if audience == "" {
		audience = "ulaundry-staff"
	}
	if lifetime == 0 {
		lifetime = 8 * time.Hour
	}
	return (security.Tokens{Secret: s.Secret, Issuer: issuer, Audience: audience, Lifetime: lifetime}).Sign(staff, at)
}
