// Package config loads and validates one startup snapshot. Errors name settings,
// never their values, since those values may contain credentials.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Database struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	SSLMode         string
	SSLRootCert     string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnectTimeout  time.Duration
	PingTimeout     time.Duration
}

type HTTP struct {
	Address           string
	StaticDir         string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type Mode string

const (
	Development Mode = "development"
	Test        Mode = "test"
	Production  Mode = "production"
)

// Cookie settings are validated now; cookie sessions are implemented separately.
type Cookie struct {
	Secret   string
	Secure   bool
	SameSite string
}

type Config struct {
	Environment    Mode
	Database       Database
	HTTP           HTTP
	JWTSecret      string
	Cookie         Cookie
	TrustedOrigins []string
	LogLevel       slog.Level
	PublicBaseURL  string
}

// System environment takes precedence over .env. Missing .env is allowed for
// deployment; unreadable or malformed files are startup failures.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, errors.New("file .env tidak dapat dibaca atau formatnya tidak valid")
	}
	return Parse(os.Getenv)
}

// Parse is pure with respect to environment/filesystem, so unit tests supply
// only synthetic settings and never depend on the developer's .env or cwd.
func Parse(getenv func(string) string) (Config, error) {
	p := parser{get: getenv}
	c := Config{
		Environment: Mode(p.text("APP_ENV", "development")),
		HTTP: HTTP{
			Address:           p.text("SERVER_ADDRESS", "127.0.0.1:8080"),
			StaticDir:         p.text("STATIC_DIR", "./frontend"),
			ReadHeaderTimeout: p.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       p.duration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:      p.duration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:       p.duration("HTTP_IDLE_TIMEOUT", 15*time.Second),
			ShutdownTimeout:   p.duration("HTTP_SHUTDOWN_TIMEOUT", 5*time.Second),
		},
		JWTSecret:     getenv("JWT_SECRET"),
		Cookie:        Cookie{Secret: getenv("COOKIE_SECRET"), SameSite: p.text("COOKIE_SAME_SITE", "lax")},
		PublicBaseURL: p.text("PUBLIC_BASE_URL", "http://localhost:8080"),
	}
	if c.Environment != Development && c.Environment != Test && c.Environment != Production {
		p.fail("APP_ENV", "harus development, test, atau production")
	}
	production := c.Environment == Production
	c.Cookie.Secure = p.boolean("COOKIE_SECURE", production)
	if production && getenv("PUBLIC_BASE_URL") == "" {
		p.fail("PUBLIC_BASE_URL", "wajib diisi pada production")
	}
	if base, err := canonicalOrigin(c.PublicBaseURL, production); err != nil {
		p.fail("PUBLIC_BASE_URL", "harus URL origin tanpa user, path, query atau fragment; production wajib HTTPS")
	} else {
		c.PublicBaseURL = base
	}
	host, port, err := net.SplitHostPort(c.HTTP.Address)
	if err != nil || !validPort(port) || strings.ContainsAny(host, " /\\\t\r\n") {
		p.fail("SERVER_ADDRESS", "harus host:port dengan port 1–65535 (IPv6 memakai kurung siku)")
	}
	if c.HTTP.ReadHeaderTimeout > c.HTTP.ReadTimeout {
		p.fail("HTTP_READ_HEADER_TIMEOUT", "tidak boleh melebihi HTTP_READ_TIMEOUT")
	}
	if c.HTTP.StaticDir == "" {
		p.fail("STATIC_DIR", "tidak boleh kosong")
	}
	p.secret("JWT_SECRET", c.JWTSecret, 32)
	p.secret("COOKIE_SECRET", c.Cookie.Secret, 32)
	if production && c.JWTSecret == c.Cookie.Secret {
		p.fail("COOKIE_SECRET", "harus berbeda dari JWT_SECRET pada production")
	}
	if c.Cookie.SameSite != "lax" && c.Cookie.SameSite != "strict" && c.Cookie.SameSite != "none" {
		p.fail("COOKIE_SAME_SITE", "harus lax, strict, atau none")
	}
	if (production || c.Cookie.SameSite == "none") && !c.Cookie.Secure {
		p.fail("COOKIE_SECURE", "wajib true pada production atau SameSite none")
	}
	switch p.text("LOG_LEVEL", "info") {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		p.fail("LOG_LEVEL", "harus debug, info, warn, atau error")
	}
	origins := getenv("TRUSTED_ORIGINS")
	if legacy := getenv("ALLOWED_ORIGIN"); legacy != "" {
		if origins != "" {
			p.fail("TRUSTED_ORIGINS", "jangan digabung dengan alias ALLOWED_ORIGIN")
		}
		origins = legacy
	}
	if origins == "" {
		origins = c.PublicBaseURL
	}
	seen := map[string]bool{}
	for _, item := range strings.Split(origins, ",") {
		origin, err := canonicalOrigin(strings.TrimSpace(item), production)
		if err != nil {
			p.fail("TRUSTED_ORIGINS", "harus daftar origin eksplisit, tanpa wildcard; production wajib HTTPS")
			continue
		}
		if !seen[origin] {
			c.TrustedOrigins = append(c.TrustedOrigins, origin)
			seen[origin] = true
		}
	}
	if !seen[c.PublicBaseURL] {
		p.fail("TRUSTED_ORIGINS", "harus mencakup origin PUBLIC_BASE_URL")
	}
	c.Database = p.database(production)
	if len(p.errs) > 0 {
		return Config{}, errors.Join(p.errs...)
	}
	return c, nil
}

type parser struct {
	get  func(string) string
	errs []error
}

func (p *parser) fail(key, message string) {
	p.errs = append(p.errs, fmt.Errorf("%s: %s", key, message))
}
func (p *parser) text(key, fallback string) string {
	if value := p.get(key); value != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
func (p *parser) duration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(p.text(key, fallback.String()))
	if err != nil || value <= 0 {
		p.fail(key, "harus durasi positif, contoh 5s atau 30m")
		return fallback
	}
	return value
}
func (p *parser) integer(key string, fallback, minimum int) int {
	value, err := strconv.Atoi(p.text(key, strconv.Itoa(fallback)))
	if err != nil || value < minimum {
		p.fail(key, "angka di luar batas yang diizinkan")
		return fallback
	}
	return value
}
func (p *parser) boolean(key string, fallback bool) bool {
	value := p.text(key, strconv.FormatBool(fallback))
	if value != "true" && value != "false" {
		p.fail(key, "harus true atau false")
		return fallback
	}
	return value == "true"
}
func (p *parser) secret(key, value string, minimum int) {
	if len(value) < minimum || value != strings.TrimSpace(value) || placeholder(value) {
		p.fail(key, fmt.Sprintf("wajib secret non-placeholder minimal %d byte tanpa spasi tepi", minimum))
	}
}
func placeholder(value string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(value))
	for _, marker := range []string{"changeme", "replace", "placeholder", "example", "your", "password", "secret123"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return len(value) > 0 && strings.Count(value, value[:1]) == len(value)
}
func validPort(value string) bool {
	port, err := strconv.Atoi(value)
	return err == nil && port >= 1 && port <= 65535
}
func canonicalOrigin(value string, requireHTTPS bool) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Opaque != "" || u.User != nil || u.Hostname() == "" ||
		(u.Scheme != "http" && u.Scheme != "https") || (requireHTTPS && u.Scheme != "https") ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.ContainsAny(u.Host, "* ,\\\t\r\n") || (u.Port() != "" && !validPort(u.Port())) || strings.HasSuffix(u.Host, ":") {
		return "", errors.New("origin tidak valid")
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host, nil
}
