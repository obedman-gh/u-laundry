package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ulaundry/internal/app"
	"ulaundry/internal/config"
	"ulaundry/internal/platform"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("Server ULaundry gagal", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: settings.LogLevel}))
	db, err := platform.OpenDatabase(ctx, settings.Database)
	if err != nil {
		return err
	}
	logger.Info("Database ULaundry siap; memulai server HTTP", "address", settings.HTTP.Address, "environment", settings.Environment)
	if err := app.New(settings, db).Run(ctx); err != nil {
		return err
	}
	logger.Info("Server HTTP dihentikan dan koneksi database ditutup")
	return nil
}
