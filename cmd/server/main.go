package main

import (
	"context"
	"errors"
	"log"
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
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	settings, err := config.Load()
	if err != nil {
		log.Print("File .env tidak dapat dimuat; menggunakan environment sistem")
	}
	db, err := platform.OpenDatabase(settings.Database)
	if err != nil {
		// Driver errors can contain deployment details; do not log the DSN.
		return errors.New("database tidak tersedia; periksa konfigurasi dan koneksi PostgreSQL")
	}
	log.Print("Database ULaundry siap; memulai server HTTP pada ", settings.HTTP.Address)
	if err := app.New(settings, db).Run(ctx); err != nil {
		return err
	}
	log.Print("Server HTTP dihentikan dan koneksi database ditutup")
	return nil
}
