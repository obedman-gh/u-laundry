package main

// Legacy demo utility; do not run against operational data.

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

	queryTabel := `
	CREATE TABLE IF NOT EXISTS keuangan (
		id_transaksi SERIAL PRIMARY KEY,
		tanggal TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		tipe VARCHAR(10) NOT NULL, -- Isinya cuma 'DEBIT' atau 'KREDIT'
		keterangan TEXT NOT NULL,
		nominal INT NOT NULL
	);`

	_, err = db.ExecContext(context.Background(), queryTabel)
	if err != nil {
		return err
	}

	// Masukin satu data pengeluaran fiktif buat test neraca
	if _, err := db.ExecContext(context.Background(), "INSERT INTO keuangan (tipe, keterangan, nominal) VALUES ('KREDIT', 'Beli Sabun Cuci 5 Liter', 45000)"); err != nil {
		return err
	}

	fmt.Println("Tabel Buku Kas & Neraca siap digunakan!")
	return nil
}
