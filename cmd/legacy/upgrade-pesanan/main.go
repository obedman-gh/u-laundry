package main

// Legacy schema utility; versioned migrations replace this separately.

import (
	"fmt"
	"log"
	"ulaundry/config"
)

func main() {
	config.ConnectDB()

	query := `
	ALTER TABLE pesanan 
	ADD COLUMN IF NOT EXISTS waktu_pesan TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	ADD COLUMN IF NOT EXISTS waktu_perkiraan_selesai TIMESTAMP,
	ADD COLUMN IF NOT EXISTS waktu_selesai_real TIMESTAMP;
	`

	_, err := config.DB.Exec(query)
	if err != nil {
		log.Fatalf("Gagal nambah kolom: %v", err)
	}
	fmt.Println("Kolom waktu_pesan, waktu_perkiraan_selesai, dan waktu_selesai_real sukses ditambahin!")
}
