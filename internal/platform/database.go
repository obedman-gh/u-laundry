package platform

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strconv"

	"ulaundry/internal/config"

	"github.com/lib/pq"
)

// databaseError retains errors.Is/As without exposing credentials from driver
// messages when an executable logs the returned error.
type databaseError struct {
	operation string
	cause     error
}

func (e *databaseError) Error() string { return "database: " + e.operation + " gagal" }
func (e *databaseError) Unwrap() error { return e.cause }
func safeDatabaseError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &databaseError{operation: operation, cause: err}
}

// OpenDatabase accepts validated settings. No PG* environment, service/passfile
// or string concatenation can override this explicit driver configuration.
func OpenDatabase(ctx context.Context, settings config.Database) (*sql.DB, error) {
	driverSettings, err := postgresConfig(settings)
	if err != nil {
		return nil, safeDatabaseError("konfigurasi", err)
	}
	connector, err := pq.NewConnectorConfig(driverSettings)
	if err != nil {
		return nil, safeDatabaseError("konfigurasi", err)
	}
	return initializeDatabase(ctx, settings, connector)
}

func postgresConfig(settings config.Database) (pq.Config, error) {
	port, err := strconv.ParseUint(settings.Port, 10, 16)
	if err != nil || port == 0 {
		return pq.Config{}, errors.New("port database tidak valid")
	}
	return pq.Config{
		Host: settings.Host, Port: uint16(port), User: settings.User, Password: settings.Password, Database: settings.Name,
		SSLMode: pq.SSLMode(settings.SSLMode), SSLRootCert: settings.SSLRootCert,
		SSLSNI: true, SSLNegotiation: pq.SSLNegotiationPostgres,
		SSLMinProtocolVersion: pq.SSLProtocolVersionTLS12,
		ConnectTimeout:        settings.ConnectTimeout, ApplicationName: "ulaundry",
		ClientEncoding: "UTF8", Datestyle: "ISO, MDY",
		MinProtocolVersion: pq.ProtocolVersion30, MaxProtocolVersion: pq.ProtocolVersion30,
	}, nil
}

func initializeDatabase(ctx context.Context, settings config.Database, connector driver.Connector) (*sql.DB, error) {
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(settings.MaxOpenConns)
	db.SetMaxIdleConns(settings.MaxIdleConns)
	db.SetConnMaxLifetime(settings.ConnMaxLifetime)
	pingCtx, cancel := context.WithTimeout(ctx, settings.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return nil, errors.Join(safeDatabaseError("ping", err), safeDatabaseError("close", db.Close()))
	}
	return db, nil
}
