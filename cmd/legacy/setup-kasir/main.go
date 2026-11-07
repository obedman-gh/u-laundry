package main

// Legacy setup utility; do not run against operational data.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"ulaundry/internal/config"
	"ulaundry/internal/platform"

	"golang.org/x/crypto/bcrypt"
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

	// 1. Bikin Tabel Kasir
	queryTabel := `
	CREATE TABLE IF NOT EXISTS kasir (
		id_kasir SERIAL PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		nama_lengkap VARCHAR(100) NOT NULL
	);`

	_, err = db.ExecContext(context.Background(), queryTabel)
	if err != nil {
		return err
	}
	fmt.Println("Tabel 'kasir' berhasil dibuat!")

	// 2. Bikin Password Rahasia
	passwordAsli := "admin123"
	// Enkripsi password pake Bcrypt (Biar hacker gak bisa baca DB lu)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(passwordAsli), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// 3. Masukin Akun Kasir ke Database
	queryInsert := `
	INSERT INTO kasir (username, password, nama_lengkap) 
	VALUES ($1, $2, $3) 
	ON CONFLICT (username) DO NOTHING;`

	_, err = db.ExecContext(context.Background(), queryInsert, "obed", string(hashedPassword), "Obed Erwin")
	if err != nil {
		return err
	}

	fmt.Println("Akun kasir berhasil dibuat!")
	fmt.Println("Username: obed")
	return nil
}
