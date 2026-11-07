// Package testutil provides opt-in disposable PostgreSQL databases for tests.
// It never loads .env or infers credentials from the application's DB settings.
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	"ulaundry/internal/config"
	"ulaundry/internal/platform"
)

func Postgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ULAUNDRY_TEST_ADMIN_DSN")
	if dsn == "" {
		t.Skip("set ULAUNDRY_TEST_ADMIN_DSN to postgres on a dedicated test cluster")
	}
	settings, err := config.ParseDatabase(func(key string) string {
		switch key {
		case "APP_ENV":
			return "test"
		case "DB_DSN":
			return dsn
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Name != "postgres" {
		t.Fatal("test admin DSN must target postgres on a dedicated test cluster")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := platform.OpenDatabase(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("ulaundry_repository_test_%x", random)
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+pq.QuoteIdentifier(name)+` TEMPLATE template0`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.ExecContext(cleanupCtx, `DROP DATABASE `+pq.QuoteIdentifier(name)); err != nil {
			t.Error(err)
		}
	})
	settings.Name = name
	db, err := platform.OpenDatabase(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}
