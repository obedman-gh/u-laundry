package main

// Legacy schema utility; versioned migrations replace this separately.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"ulaundry/internal/config"
	"ulaundry/internal/platform"
)

func main() {
	if err := run(); err != nil {
		log.Print("Utilitas legacy gagal; periksa konfigurasi dan database")
		os.Exit(1)
	}
}

func run() (result error) {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	if settings.Environment == config.Production {
		return errors.New("utilitas legacy dilarang pada production")
	}
	db, err := platform.OpenDatabase(context.Background(), settings.Database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()

	query := `
	ALTER TABLE pesanan 
	ADD COLUMN IF NOT EXISTS waktu_pesan TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	ADD COLUMN IF NOT EXISTS waktu_perkiraan_selesai TIMESTAMP,
	ADD COLUMN IF NOT EXISTS waktu_selesai_real TIMESTAMP;
	`

	_, err = db.ExecContext(context.Background(), query)
	if err != nil {
		return err
	}
	fmt.Println("Kolom waktu_pesan, waktu_perkiraan_selesai, dan waktu_selesai_real sukses ditambahin!")
	return nil
}
