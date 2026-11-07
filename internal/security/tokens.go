// Package security owns token/session policy and credential-free security events.
package security

import (
	"crypto/rand"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"time"
	"ulaundry/internal/domain"
)

var ErrUnauthorized = errors.New("Sesi tidak valid atau telah berakhir.")
var ErrUnavailable = errors.New("Layanan autentikasi tidak tersedia.")

type Claims struct {
	jwt.RegisteredClaims
	Username string      `json:"username"`
	Role     domain.Role `json:"role"`
}
type Tokens struct {
	Secret, Issuer, Audience string
	Lifetime                 time.Duration
}

func (p Tokens) Valid() bool {
	return len(p.Secret) >= 32 && p.Issuer != "" && p.Audience != "" && p.Lifetime >= 15*time.Minute && p.Lifetime <= 12*time.Hour && p.Lifetime%time.Second == 0
}
func (p Tokens) Sign(staff domain.Staff, at time.Time) (string, error) {
	if !p.Valid() || at.IsZero() || !identifier(staff.ID) || !identifier(staff.Username) || !role(staff.Role) {
		return "", ErrUnavailable
	}
	at = at.UTC().Truncate(time.Second)
	c := Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: p.Issuer, Audience: jwt.ClaimStrings{p.Audience}, Subject: staff.ID, ID: rand.Text(), IssuedAt: jwt.NewNumericDate(at), ExpiresAt: jwt.NewNumericDate(at.Add(p.Lifetime))}, Username: staff.Username, Role: staff.Role}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(p.Secret))
	if err != nil {
		return "", ErrUnavailable
	}
	return signed, nil
}
func (p Tokens) Parse(raw string, at time.Time) (Claims, error) {
	var c Claims
	if !p.Valid() {
		return c, ErrUnavailable
	}
	if len(raw) == 0 || len(raw) > 4096 {
		return c, ErrUnauthorized
	}
	token, err := jwt.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}
		return []byte(p.Secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(p.Issuer), jwt.WithAudience(p.Audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(func() time.Time { return at }))
	if err != nil || !token.Valid || c.IssuedAt == nil || c.ExpiresAt == nil || c.IssuedAt.After(at) || !c.ExpiresAt.After(c.IssuedAt.Time) || c.ExpiresAt.Sub(c.IssuedAt.Time) > p.Lifetime || !identifier(c.Subject) || !identifier(c.Username) || len(c.ID) < 26 || len(c.ID) > 64 || !identifier(c.ID) || !role(c.Role) {
		return Claims{}, ErrUnauthorized
	}
	return c, nil
}
func identifier(s string) bool {
	if len(s) < 1 || len(s) > 100 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
func role(r domain.Role) bool { return r == domain.Owner || r == domain.Admin || r == domain.Cashier }
