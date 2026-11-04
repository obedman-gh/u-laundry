package main

import (
	"fmt"
	"log"
	"net/http"

	"ulaundry/config"
	"ulaundry/handlers"
)

func main() {
	// 1. Inisiasi Database
	config.ConnectDB()
	fmt.Println("Database Ulaundry Ready!")

	// 2. Setup Router
	mux := http.NewServeMux()

	// --- TAMBAHIN BARIS INI ---
	// Menyajikan file HTML statis dari folder "frontend" pas user buka http://localhost:8080/
	fs := http.FileServer(http.Dir("./frontend"))
	mux.Handle("/", fs)
	// -------------------------

	// Route Public (Siapa aja boleh akses, termasuk customer buat ngecek)
	mux.HandleFunc("GET /api/pesanan", handlers.BukaPintuCORS(handlers.GetSemuaPesanan))
	// Route buat Filter by Status (Harus ditaruh SEBELUM route {id} biar nggak bentrok kalau ada route id nanti)
	mux.HandleFunc("GET /api/pesanan/status", handlers.BukaPintuCORS(handlers.GetPesananByStatus))
	// Route buat Login Kasir
	mux.HandleFunc("POST /api/login", handlers.BukaPintuCORS(handlers.Login))
	// Route buat liat dan cetak struk (Public)
	mux.HandleFunc("GET /api/pesanan/{id}/struk", handlers.BukaPintuCORS(handlers.CetakStruk))

	// Route Private (KHUSUS KASIR YANG UDAH LOGIN)
	// Perhatiin bungkusannya lapis dua: BukaPintuCORS(ValidasiJWT(FungsiUtama))
	mux.HandleFunc("POST /api/pesanan", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.BikinPesananBaru)))
	// Route buat Update (Pake method PUT)
	mux.HandleFunc("PUT /api/pesanan/{id}", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.UpdateStatusPesanan)))
	// Tambahkan di bawah route PUT pesanan
	mux.HandleFunc("GET /api/laporan/neraca", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.GetNeraca)))

	// 3. Jalankan Server
	fmt.Println("Server API Ulaundry Berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
