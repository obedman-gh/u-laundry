package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testJWT = "unit-test-only-jwt-key-7392046185-abcd"
const testCookie = "unit-test-only-cookie-key-5831927406-efgh"

func developmentEnv() map[string]string {
	return map[string]string{"JWT_SECRET": testJWT, "COOKIE_SECRET": testCookie}
}
func productionEnv() map[string]string {
	env := developmentEnv()
	for k, v := range map[string]string{
		"APP_ENV": "production", "PUBLIC_BASE_URL": "https://laundry.invalid",
		"DB_HOST": "db.internal", "DB_USER": "fixture", "DB_NAME": "fixture",
		"DB_PASS": "unit-test-db-key-482619", "DB_SSLMODE": "verify-full", "DB_SSLROOTCERT": "/fixture/ca.crt",
	} {
		env[k] = v
	}
	return env
}
func parseMap(env map[string]string) (Config, error) {
	return Parse(func(key string) string { return env[key] })
}

func TestValidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		mode Mode
	}{
		{"development", developmentEnv(), Development},
		{"production", productionEnv(), Production},
		{"test", func() map[string]string { e := developmentEnv(); e["APP_ENV"] = "test"; return e }(), Test},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseMap(tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if c.Environment != tc.mode || len(c.TrustedOrigins) != 1 || c.TrustedOrigins[0] != c.PublicBaseURL {
				t.Fatal("mode or same-origin default missing")
			}
			if c.Cookie.Secure != (tc.mode == Production) || c.Cookie.SameSite != "lax" {
				t.Fatal("unsafe cookie defaults")
			}
			if c.Database.MaxOpenConns != 10 || c.Database.MaxIdleConns != 5 || c.Database.PingTimeout != 5*time.Second {
				t.Fatal("database defaults missing")
			}
			if tc.mode == Development && c.HTTP.Address != "127.0.0.1:8080" {
				t.Fatal("development must bind loopback by default")
			}
		})
	}
}

func TestConfigurationOverrides(t *testing.T) {
	env := developmentEnv()
	for k, v := range map[string]string{
		"SERVER_ADDRESS": "[::1]:9090", "PUBLIC_BASE_URL": "http://[::1]:9090/", "STATIC_DIR": "./assets",
		"HTTP_READ_HEADER_TIMEOUT": "2s", "HTTP_READ_TIMEOUT": "11s", "HTTP_WRITE_TIMEOUT": "12s",
		"HTTP_IDLE_TIMEOUT": "20s", "HTTP_SHUTDOWN_TIMEOUT": "8s", "LOG_LEVEL": "warn",
		"DB_MAX_OPEN_CONNS": "3", "DB_MAX_IDLE_CONNS": "0", "DB_CONN_MAX_LIFETIME": "7m", "DB_CONNECT_TIMEOUT": "2s", "DB_PING_TIMEOUT": "3s",
		"COOKIE_SECURE": "true", "COOKIE_SAME_SITE": "strict",
		"TRUSTED_ORIGINS": "http://[::1]:9090, https://EXTRA.invalid:443/",
	} {
		env[k] = v
	}
	c, err := parseMap(env)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTP.ReadHeaderTimeout != 2*time.Second || c.HTTP.ReadTimeout != 11*time.Second || c.HTTP.WriteTimeout != 12*time.Second || c.HTTP.IdleTimeout != 20*time.Second || c.HTTP.ShutdownTimeout != 8*time.Second || c.HTTP.StaticDir != "./assets" {
		t.Fatal("HTTP overrides not applied")
	}
	if c.LogLevel != slog.LevelWarn || c.Database.ConnMaxLifetime != 7*time.Minute || c.Database.MaxOpenConns != 3 || c.Database.MaxIdleConns != 0 || c.Database.ConnectTimeout != 2*time.Second || c.Database.PingTimeout != 3*time.Second {
		t.Fatal("pool/log overrides not applied")
	}
	if len(c.TrustedOrigins) != 2 || c.TrustedOrigins[1] != "https://extra.invalid" || !c.Cookie.Secure || c.Cookie.SameSite != "strict" {
		t.Fatal("origin/cookie normalization missing")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"APP_ENV", "prod"}, {"SERVER_ADDRESS", "bad"}, {"SERVER_ADDRESS", "localhost:0"},
		{"HTTP_READ_HEADER_TIMEOUT", "0s"}, {"HTTP_READ_HEADER_TIMEOUT", "20s"}, {"HTTP_READ_TIMEOUT", "bad"},
		{"HTTP_WRITE_TIMEOUT", "-1s"}, {"HTTP_IDLE_TIMEOUT", "0"}, {"HTTP_SHUTDOWN_TIMEOUT", "wrong"},
		{"JWT_SECRET", ""}, {"JWT_SECRET", "short"}, {"JWT_SECRET", strings.Repeat("x", 40)},
		{"JWT_SECRET", "your_super_secret_jwt_key_replace_this_value"}, {"COOKIE_SECRET", "short"},
		{"COOKIE_SECURE", "maybe"}, {"COOKIE_SAME_SITE", "invalid"}, {"LOG_LEVEL", "trace"},
		{"DB_PORT", "65536"}, {"DB_HOST", "host sslmode=disable"}, {"DB_SSLMODE", "prefer"},
		{"DB_MAX_OPEN_CONNS", "0"}, {"DB_MAX_IDLE_CONNS", "-1"}, {"DB_MAX_IDLE_CONNS", "11"},
		{"DB_CONN_MAX_LIFETIME", "0s"}, {"DB_CONNECT_TIMEOUT", "6s"}, {"DB_PING_TIMEOUT", "-1s"},
		{"TRUSTED_ORIGINS", "*"}, {"TRUSTED_ORIGINS", "http://*.invalid"}, {"TRUSTED_ORIGINS", "null"},
		{"PUBLIC_BASE_URL", "http://user:credential@localhost"}, {"PUBLIC_BASE_URL", "http://localhost/path"},
		{"PUBLIC_BASE_URL", "http://localhost?x=1"}, {"PUBLIC_BASE_URL", "http://localhost#frag"},
		{"PUBLIC_BASE_URL", "http://localhost:99999"}, {"TRUSTED_ORIGINS", "http://other.invalid"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			env := developmentEnv()
			env[tc.key] = tc.value
			if _, err := parseMap(env); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
	for _, tc := range []struct{ key, value string }{
		{"JWT_SECRET", ""}, {"JWT_SECRET", "short"}, {"COOKIE_SECRET", ""}, {"COOKIE_SECRET", testJWT},
		{"COOKIE_SECURE", "false"}, {"PUBLIC_BASE_URL", ""}, {"PUBLIC_BASE_URL", "http://laundry.invalid"},
		{"DB_HOST", ""}, {"DB_USER", ""}, {"DB_NAME", ""}, {"DB_PASS", ""}, {"DB_PASS", "short"},
		{"DB_SSLMODE", "disable"}, {"DB_SSLMODE", "require"}, {"DB_SSLMODE", "verify-ca"}, {"DB_SSLROOTCERT", ""},
		{"TRUSTED_ORIGINS", "*"}, {"TRUSTED_ORIGINS", "https://laundry.invalid,http://other.invalid"},
	} {
		t.Run("production/"+tc.key+"/"+tc.value, func(t *testing.T) {
			env := productionEnv()
			env[tc.key] = tc.value
			if _, err := parseMap(env); err == nil {
				t.Fatal("unsafe production configuration accepted")
			}
		})
	}
}

func TestLegacyOriginAliasAndCookiePolicy(t *testing.T) {
	env := developmentEnv()
	env["ALLOWED_ORIGIN"] = "http://localhost:8080"
	if _, err := parseMap(env); err != nil {
		t.Fatal(err)
	}
	env["TRUSTED_ORIGINS"] = "http://localhost:8080"
	if _, err := parseMap(env); err == nil {
		t.Fatal("ambiguous origins accepted")
	}
	delete(env, "TRUSTED_ORIGINS")
	env["COOKIE_SAME_SITE"] = "none"
	if _, err := parseMap(env); err == nil {
		t.Fatal("insecure SameSite none accepted")
	}
	env["COOKIE_SECURE"] = "true"
	if _, err := parseMap(env); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseURL(t *testing.T) {
	env := developmentEnv()
	env["DB_DSN"] = "postgresql://fixture:p%40ss%20%27%5Cword@127.0.0.1:5433/fixture_db?sslmode=disable"
	c, err := parseMap(env)
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.Password != "p@ss '\\word" || c.Database.Port != "5433" || c.Database.Name != "fixture_db" {
		t.Fatal("DSN credential escaping or port lost")
	}
	for _, raw := range []string{
		"host=localhost password=credential", "postgres://user:credential@%zz/fixture",
		"postgres://user:credential@host/fixture?host=attacker", "postgres://user:credential@host/fixture?sslmode=disable&sslmode=verify-full",
		"postgres://user:credential@host/fixture?sslmode=disable;user=other", "postgres://host/fixture", "postgres://user@host/",
	} {
		env["DB_DSN"] = raw
		_, err := parseMap(env)
		if err == nil || strings.Contains(err.Error(), "credential") {
			t.Fatal("bad DSN accepted or error leaked credential")
		}
	}
	env["DB_DSN"] = "postgres://fixture:unit-test-db-key-482619@db.internal/fixture?sslmode=verify-full&sslrootcert=%2Ffixture%2Fca.crt"
	env["APP_ENV"], env["PUBLIC_BASE_URL"] = "production", "https://laundry.invalid"
	if _, err := parseMap(env); err != nil {
		t.Fatal(err)
	}
	env["DB_HOST"] = "db.internal"
	if _, err := parseMap(env); err == nil {
		t.Fatal("ambiguous DSN/components accepted")
	}
}
