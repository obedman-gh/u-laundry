// Package app owns HTTP and database lifecycle. It does not load environment
// variables or connect to PostgreSQL; dependencies are supplied by the executable.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"ulaundry/internal/config"
	apphttp "ulaundry/internal/http"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/http/middleware"
)

type Database interface {
	handler.Database
	io.Closer
}

type App struct {
	server          *http.Server
	db              Database
	shutdownTimeout time.Duration
}

// New takes ownership of db. Run or Serve closes it after HTTP has stopped.
func New(settings config.Config, db Database) *App {
	h := handler.New(db, settings.JWTSecret)
	m := middleware.New(settings.JWTSecret, settings.AllowedOrigin)
	return &App{
		server: &http.Server{
			Addr:         settings.HTTP.Address,
			Handler:      apphttp.NewRouter(h, m, http.Dir(settings.HTTP.StaticDir)),
			ReadTimeout:  settings.HTTP.ReadTimeout,
			WriteTimeout: settings.HTTP.WriteTimeout,
			IdleTimeout:  settings.HTTP.IdleTimeout,
		},
		db: db, shutdownTimeout: settings.HTTP.ShutdownTimeout,
	}
}

func (a *App) closeDatabase() error {
	if err := a.db.Close(); err != nil {
		return fmt.Errorf("gagal menutup database: %w", err)
	}
	return nil
}

func (a *App) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return errors.Join(fmt.Errorf("gagal membuka listener HTTP: %w", err), a.closeDatabase())
	}
	return a.Serve(ctx, listener)
}

// Serve accepts an owned listener, allowing lifecycle checks on an isolated
// loopback address. Cancellation drains requests within the existing grace time.
func (a *App) Serve(ctx context.Context, listener net.Listener) (result error) {
	defer func() { result = errors.Join(result, a.closeDatabase()) }()
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
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		// A timed-out graceful shutdown must not leave the listener running.
		closeErr := a.server.Close()
		return errors.Join(fmt.Errorf("shutdown HTTP tidak selesai: %w", err), closeErr, <-serveErrors)
	}
	return <-serveErrors
}
