package main

import (
	"fmt"
	"log"

	"ulaundry/config" // sesuaikan dengan nama module lu

	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Koneksi ke DB
	config.ConnectDB()

	// 1. Bikin Tabel Kasir
	queryTabel := `
	CREATE TABLE IF NOT EXISTS kasir (
		id_kasir SERIAL PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		nama_lengkap VARCHAR(100) NOT NULL
	);`

	_, err := config.DB.Exec(queryTabel)
	if err != nil {
		log.Fatalf("Gagal bikin tabel kasir: %v", err)
	}
	fmt.Println("Tabel 'kasir' berhasil dibuat!")

	// 2. Bikin Password Rahasia
	passwordAsli := "admin123"
	// Enkripsi password pake Bcrypt (Biar hacker gak bisa baca DB lu)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(passwordAsli), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Gagal enkripsi password: %v", err)
	}

	// 3. Masukin Akun Kasir ke Database
	queryInsert := `
	INSERT INTO kasir (username, password, nama_lengkap) 
	VALUES ($1, $2, $3) 
	ON CONFLICT (username) DO NOTHING;`

	_, err = config.DB.Exec(queryInsert, "obed", string(hashedPassword), "Obed Erwin")
	if err != nil {
		log.Fatalf("Gagal insert akun kasir: %v", err)
	}

	fmt.Println("Akun kasir berhasil dibuat!")
	fmt.Println("Username: obed")
	fmt.Println("Password:", passwordAsli)
}
