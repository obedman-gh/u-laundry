package main

// Legacy demo utility; do not run against operational data.

import (
	"fmt"
	"log"
	"ulaundry/config"
)

func main() {
	config.ConnectDB()

	queryTabel := `
	CREATE TABLE IF NOT EXISTS keuangan (
		id_transaksi SERIAL PRIMARY KEY,
		tanggal TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		tipe VARCHAR(10) NOT NULL, -- Isinya cuma 'DEBIT' atau 'KREDIT'
		keterangan TEXT NOT NULL,
		nominal INT NOT NULL
	);`

	_, err := config.DB.Exec(queryTabel)
	if err != nil {
		log.Fatalf("Gagal bikin tabel keuangan: %v", err)
	}

	// Masukin satu data pengeluaran fiktif buat test neraca
	config.DB.Exec("INSERT INTO keuangan (tipe, keterangan, nominal) VALUES ('KREDIT', 'Beli Sabun Cuci 5 Liter', 45000)")

	fmt.Println("Tabel Buku Kas & Neraca siap digunakan!")
}
