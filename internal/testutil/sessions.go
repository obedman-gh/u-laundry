package testutil

import (
	"context"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"sync"
	"time"
	"ulaundry/internal/security"
)

// SessionStore is a synthetic, concurrency-safe store, never used by cmd/server.
type SessionStore struct {
	mu                sync.Mutex
	Records           map[[32]byte]security.Record
	Events            []security.Event
	AllowUnregistered bool
	Failure           error
}

func (s *SessionStore) Create(ctx context.Context, r security.Record, e security.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != nil {
		return s.Failure
	}
	if s.Records == nil {
		s.Records = map[[32]byte]security.Record{}
	}
	s.Records[r.Hash] = r
	s.Events = append(s.Events, e)
	return ctx.Err()
}
func (s *SessionStore) Check(ctx context.Context, r security.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != nil {
		return s.Failure
	}
	if s.AllowUnregistered {
		return ctx.Err()
	}
	saved, ok := s.Records[r.Hash]
	if !ok || saved.UserID != r.UserID || saved.Username != r.Username || saved.Role != r.Role || !saved.ExpiresAt.Equal(r.ExpiresAt) {
		return security.ErrUnauthorized
	}
	return ctx.Err()
}
func (s *SessionStore) Revoke(ctx context.Context, r security.Record, e security.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != nil {
		return s.Failure
	}
	delete(s.Records, r.Hash)
	s.Events = append(s.Events, e)
	return ctx.Err()
}
func (s *SessionStore) Audit(ctx context.Context, e security.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != nil {
		return s.Failure
	}
	s.Events = append(s.Events, e)
	return ctx.Err()
}

// TestSessions isolates HTTP/JWT tests from repository persistence, separately
// verified by auth integration tests. It is not production session storage.
func TestSessions(secret string) *security.Manager {
	return &security.Manager{Tokens: security.Tokens{Secret: secret, Issuer: "ulaundry", Audience: "ulaundry-staff", Lifetime: 8 * time.Hour}, Store: &SessionStore{AllowUnregistered: true}, Secret: "synthetic-cookie-hmac-key-32bytes-only", Origin: "https://fixture.invalid", SameSite: http.SameSiteLaxMode, Limiter: &security.Limiter{}}
}
func TestClaims(subject string) jwt.MapClaims {
	at := time.Now()
	return jwt.MapClaims{"sub": subject, "username": "staff.test", "role": "cashier", "iss": "ulaundry", "aud": "ulaundry-staff", "iat": at.Unix(), "exp": at.Add(time.Hour).Unix(), "jti": "synthetic-session-identifier-00001"}
}
