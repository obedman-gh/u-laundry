// Package app owns HTTP and database lifecycle. It does not load environment
// variables or connect to PostgreSQL; dependencies are supplied by the executable.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"ulaundry/internal/config"
	apphttp "ulaundry/internal/http"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/http/middleware"
	"ulaundry/internal/telemetry"
)

type Database interface {
	io.Closer
}

type App struct {
	server          *http.Server
	db              Database
	shutdownTimeout time.Duration
	draining        atomic.Bool
	logger          *slog.Logger
}

// New takes ownership of db. Run or Serve closes it after HTTP has stopped.
func New(settings config.Config, db Database, use handler.Dependencies) *App {
	if use.Logger == nil {
		use.Logger = telemetry.NewLogger(io.Discard, settings.LogLevel)
	}
	if use.Metrics == nil {
		use.Metrics = telemetry.NewRegistry()
	}
	a := &App{db: db, shutdownTimeout: settings.HTTP.ShutdownTimeout, logger: use.Logger}
	use.Draining = a.draining.Load
	use.ReadinessTimeout = settings.HTTP.ReadinessTimeout
	if stats, ok := db.(interface{ Stats() sql.DBStats }); ok {
		use.PoolStats = func() telemetry.Pool { return telemetry.DBPool(stats.Stats()) }
	}
	h := handler.New(use)
	m := middleware.New(settings.TrustedOrigins, use.Sessions).WithObservability(use.Logger, use.Metrics)
	a.server = &http.Server{
		Addr:              settings.HTTP.Address,
		Handler:           apphttp.NewRouter(h, m, http.Dir(settings.HTTP.StaticDir)),
		ReadTimeout:       settings.HTTP.ReadTimeout,
		ReadHeaderTimeout: settings.HTTP.ReadHeaderTimeout,
		WriteTimeout:      settings.HTTP.WriteTimeout,
		IdleTimeout:       settings.HTTP.IdleTimeout,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(telemetry.ServerErrorWriter{Logger: use.Logger}, "", 0),
	}
	return a
}

func (a *App) closeDatabase() error {
	if err := a.db.Close(); err != nil {
		a.logger.Error("database.close_failed", "error_class", "database")
		return fmt.Errorf("gagal menutup database: %w", err)
	}
	return nil
}

func (a *App) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		a.draining.Store(true)
		a.logger.Error("server.listen_failed", "error_class", "listener")
		return errors.Join(fmt.Errorf("gagal membuka listener HTTP: %w", err), a.closeDatabase())
	}
	return a.Serve(ctx, listener)
}

// Serve accepts an owned listener, allowing lifecycle checks on an isolated
// loopback address. Cancellation drains requests within the existing grace time.
func (a *App) Serve(ctx context.Context, listener net.Listener) (result error) {
	defer func() {
		a.draining.Store(true)
		result = errors.Join(result, a.closeDatabase())
		if result != nil {
			a.logger.Error("server.stopped", "error_class", "shutdown")
		} else {
			a.logger.Info("server.stopped", "error_class", "none")
		}
	}()
	serveErrors := make(chan error, 1)
	go func() {
		err := a.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErrors <- err
	}()

	select {
	case err := <-serveErrors:
		if err != nil {
			a.logger.Error("server.serve_failed", "error_class", "listener")
		}
		a.draining.Store(true)
		return errors.Join(err, a.server.Close())
	case <-ctx.Done():
	}
	a.draining.Store(true)
	a.logger.Info("server.stopping", "error_class", "none")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.Warn("server.shutdown_forced", "error_class", "timeout")
		// A timed-out graceful shutdown must not leave the listener running.
		closeErr := a.server.Close()
		return errors.Join(fmt.Errorf("shutdown HTTP tidak selesai: %w", err), closeErr, <-serveErrors)
	}
	return <-serveErrors
}
