package security

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"time"
)

type Record struct {
	Hash                   [32]byte
	UserID, Username, Role string
	Fingerprint            [32]byte
	IssuedAt, ExpiresAt    time.Time
}
type Event struct {
	Kind, ActorID, RequestID string
	At                       time.Time
}
type Store interface {
	Create(context.Context, Record, Event) error
	Check(context.Context, Record) error
	Revoke(context.Context, Record, Event) error
	Audit(context.Context, Event) error
}
type Manager struct {
	Tokens         Tokens
	Store          Store
	Secret, Origin string
	Secure         bool
	SameSite       http.SameSite
	Now            func() time.Time
	Limiter        *Limiter
	PeerLimit      int
	PeerWindow     time.Duration
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}
func (m *Manager) CookieName() string {
	if m.Secure {
		return "__Host-ulaundry_session"
	}
	return "ulaundry_session"
}
func (m *Manager) cookie(value string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{Name: m.CookieName(), Value: value, Path: "/", HttpOnly: true, Secure: m.Secure, SameSite: m.SameSite, Expires: expires, MaxAge: maxAge}
}
func (m *Manager) SetCookie(w http.ResponseWriter, raw string, c Claims) {
	http.SetCookie(w, m.cookie(raw, c.ExpiresAt.Time, int(c.ExpiresAt.Sub(m.now()).Seconds())))
}
func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, m.cookie("", time.Unix(1, 0).UTC(), -1))
}
func record(c Claims) Record {
	return Record{Hash: sha256.Sum256([]byte(c.ID)), UserID: c.Subject, Username: c.Username, Role: string(c.Role), IssuedAt: c.IssuedAt.Time, ExpiresAt: c.ExpiresAt.Time}
}
func (m *Manager) Start(ctx context.Context, raw string, fingerprint [32]byte, requestID string) (Claims, error) {
	c, err := m.Tokens.Parse(raw, m.now())
	if err != nil {
		return c, err
	}
	if m.Store == nil {
		return Claims{}, ErrUnavailable
	}
	r := record(c)
	r.Fingerprint = fingerprint
	if err := m.Store.Create(ctx, r, Event{Kind: "login_success", ActorID: c.Subject, RequestID: requestID, At: m.now()}); err != nil {
		return Claims{}, err
	}
	return c, nil
}
func (m *Manager) Authenticate(r *http.Request) (Claims, error) {
	if m == nil || m.Store == nil {
		return Claims{}, ErrUnavailable
	}
	if len(r.Header.Values("Authorization")) != 0 {
		return Claims{}, ErrUnauthorized
	}
	cookies := r.CookiesNamed(m.CookieName())
	if len(cookies) != 1 {
		return Claims{}, ErrUnauthorized
	}
	c, err := m.Tokens.Parse(cookies[0].Value, m.now())
	if err != nil {
		return Claims{}, err
	}
	if err := m.Store.Check(r.Context(), record(c)); err != nil {
		return Claims{}, err
	}
	return c, nil
}
func (m *Manager) csrf(c Claims) string {
	h := hmac.New(sha256.New, []byte(m.Secret))
	h.Write([]byte("csrf\x00" + c.ID))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (m *Manager) CSRF(c Claims) string { return m.csrf(c) }
func (m *Manager) SafeOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origins := r.Header.Values("Origin")
	return len(origins) == 1 && origins[0] == m.Origin
}
func (m *Manager) VerifyCSRF(r *http.Request, c Claims) bool {
	values := r.Header.Values("X-CSRF-Token")
	if len(values) != 1 || len(values[0]) != 43 || !m.SafeOrigin(r) {
		return false
	}
	return hmac.Equal([]byte(values[0]), []byte(m.csrf(c)))
}
func (m *Manager) Logout(ctx context.Context, c Claims, requestID string) error {
	return m.Store.Revoke(ctx, record(c), Event{Kind: "logout", ActorID: c.Subject, RequestID: requestID, At: m.now()})
}
func (m *Manager) Audit(ctx context.Context, kind, requestID string) error {
	if m.Store == nil {
		return ErrUnavailable
	}
	return m.Store.Audit(ctx, Event{Kind: kind, RequestID: requestID, At: m.now()})
}
func (m *Manager) AllowLogin(ctx context.Context, r *http.Request, username, requestID string) (bool, time.Duration, error) {
	if m.Limiter == nil {
		return false, 0, ErrUnavailable
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if ip := net.ParseIP(peer); ip != nil {
		peer = ip.String()
	}
	key := func(kind, value string) string {
		h := hmac.New(sha256.New, []byte(m.Secret))
		h.Write([]byte(kind + "\x00" + value))
		return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	}
	n, window := m.PeerLimit, m.PeerWindow
	if n < 1 {
		n = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	allowed, retry, notify := m.Limiter.Take(m.now(), Limit{Key: "global", Count: 100, Window: time.Minute}, Limit{Key: key("peer", peer), Count: n, Window: window}, Limit{Key: key("account", strings.ToLower(username)), Count: 10, Window: 15 * time.Minute})
	if notify {
		if err := m.Audit(ctx, "login_limited", requestID); err != nil {
			return false, retry, err
		}
	}
	return allowed, retry, nil
}
