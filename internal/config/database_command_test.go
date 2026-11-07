package config

import "testing"

func TestDatabaseCommandPolicy(t *testing.T) {
	// Operational commands need no JWT/cookie secrets and no developer .env.
	if _, err := ParseDatabase(func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]string{
		{"APP_ENV": "invalid"},
		{"APP_ENV": "production"},
		{"DB_DSN": "postgres://u@localhost/db", "DB_NAME": "conflict"},
		{"DB_SSLMODE": "prefer"},
	} {
		if _, err := ParseDatabase(func(key string) string { return values[key] }); err == nil {
			t.Fatal("unsafe database configuration accepted")
		}
	}
}
