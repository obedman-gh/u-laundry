package platform

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ulaundry/internal/config"

	"github.com/lib/pq"
)

type connectionState struct {
	opened   atomic.Int32
	closed   atomic.Int32
	deadline atomic.Bool
	block    bool
	closeErr error
}
type testConnector struct{ state *connectionState }

func (c testConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.state.opened.Add(1)
	return &testConnection{state: c.state}, nil
}
func (testConnector) Driver() driver.Driver { return testDriver{} }

type testDriver struct{}

func (testDriver) Open(string) (driver.Conn, error) { return nil, errors.New("connector required") }

type testConnection struct{ state *connectionState }

func (*testConnection) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not used") }
func (*testConnection) Begin() (driver.Tx, error)           { return nil, errors.New("not used") }
func (c *testConnection) Close() error                      { c.state.closed.Add(1); return c.state.closeErr }
func (c *testConnection) Ping(ctx context.Context) error {
	_, hasDeadline := ctx.Deadline()
	c.state.deadline.Store(hasDeadline)
	if c.state.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func poolSettings() config.Database {
	return config.Database{MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute, PingTimeout: time.Second}
}

func TestDatabasePoolAndClose(t *testing.T) {
	state := &connectionState{}
	db, err := initializeDatabase(context.Background(), poolSettings(), testConnector{state})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if !state.deadline.Load() || db.Stats().MaxOpenConnections != 2 {
		t.Fatal("ping deadline or max-open limit not applied")
	}
	first, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if db.Stats().Idle != 1 || state.opened.Load() != 2 {
		t.Fatal("max-idle limit not applied")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if state.closed.Load() != 2 {
		t.Fatal("database close leaked connections")
	}
}

func TestPingDeadlineFailureClosesPoolAndRedactsError(t *testing.T) {
	closeErr := errors.New("synthetic-credential-that-must-not-be-logged")
	state := &connectionState{block: true, closeErr: closeErr}
	settings := poolSettings()
	settings.PingTimeout = 20 * time.Millisecond
	db, err := initializeDatabase(context.Background(), settings, testConnector{state})
	if db != nil || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, closeErr) || state.closed.Load() != 1 {
		t.Fatalf("deadline/cleanup cause lost: %v", err)
	}
	if strings.Contains(err.Error(), closeErr.Error()) {
		t.Fatal("driver error leaked credential")
	}
}

func TestDatabaseHonorsCanceledStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state := &connectionState{}
	db, err := initializeDatabase(ctx, poolSettings(), testConnector{state})
	if db != nil || !errors.Is(err, context.Canceled) || state.opened.Load() != 0 {
		t.Fatalf("startup cancellation ignored: %v", err)
	}
}

func TestExplicitPostgresConfiguration(t *testing.T) {
	t.Setenv("PGHOST", "untrusted.invalid")
	t.Setenv("PGSSLMODE", "disable")
	t.Setenv("PGOPTIONS", "-c search_path=untrusted")
	settings := config.Database{Host: "db.internal", Port: "5432", User: "fixture", Password: "spaces '\\ @", Name: "fixture_db", SSLMode: "verify-full", SSLRootCert: "/fixture/ca.crt", ConnectTimeout: 2 * time.Second}
	c, err := postgresConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != settings.Host || c.Password != settings.Password || c.Database != settings.Name || c.SSLMode != pq.SSLModeVerifyFull || c.SSLRootCert != settings.SSLRootCert || c.ConnectTimeout != settings.ConnectTimeout || c.Options != "" || c.Passfile != "" || c.Service != "" {
		t.Fatal("typed driver settings changed or inherited ambient configuration")
	}
	if c.SSLMinProtocolVersion != pq.SSLProtocolVersionTLS12 || c.ClientEncoding != "UTF8" || !c.SSLSNI {
		t.Fatal("driver TLS/encoding defaults missing")
	}
}
