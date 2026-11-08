package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"net/http"
	"ulaundry/internal/app"
	"ulaundry/internal/config"
	"ulaundry/internal/http/handler"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/security"
	"ulaundry/internal/service"
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
	store := postgres.New(db)
	uow := service.WithAccessAudit(store, store)
	runtime := service.Runtime{Clock: platform.SystemClock{}, IDs: platform.RandomIDs{}, Tokens: platform.RandomTokens{}}
	use := handler.Dependencies{
		Staff:     service.NewStaffManagement(uow, runtime, platform.Passwords{}),
		Customers: service.NewCustomers(uow, runtime), Catalog: service.NewCatalog(uow, runtime), Orders: service.NewOrders(uow, runtime), Payments: service.NewPayments(uow, runtime),
		Tracking: service.NewTracking(uow), Reports: service.NewReports(uow, runtime), Auth: service.NewAuth(uow, runtime, platform.Passwords{}, platform.JWTIssuer{Secret: settings.JWTSecret, Issuer: settings.JWTIssuer, Audience: settings.JWTAudience, Lifetime: settings.SessionLifetime}), Health: service.NewHealth(db),
	}
	sameSite := http.SameSiteLaxMode
	if settings.Cookie.SameSite == "strict" {
		sameSite = http.SameSiteStrictMode
	} else if settings.Cookie.SameSite == "none" {
		sameSite = http.SameSiteNoneMode
	}
	use.Sessions = &security.Manager{Tokens: security.Tokens{Secret: settings.JWTSecret, Issuer: settings.JWTIssuer, Audience: settings.JWTAudience, Lifetime: settings.SessionLifetime}, Store: store, Secret: settings.Cookie.Secret, Origin: settings.PublicBaseURL, Secure: settings.Cookie.Secure, SameSite: sameSite, Limiter: &security.Limiter{}, PeerLimit: settings.LoginLimit, PeerWindow: settings.LoginWindow}
	if err := app.New(settings, db, use).Run(ctx); err != nil {
		return err
	}
	logger.Info("Server HTTP dihentikan dan koneksi database ditutup")
	return nil
}
