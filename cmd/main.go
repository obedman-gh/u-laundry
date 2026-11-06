package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ulaundry/config"
	"ulaundry/handlers"
)

func main() {
	// 1. Inisiasi Database
	config.ConnectDB()
	fmt.Println("Database Ulaundry Ready!")

	// 2. Setup Router
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./frontend"))
	mux.Handle("/", fs)

	// Route Public & Health Check
	mux.HandleFunc("GET /api/health", handlers.BukaPintuCORS(handlers.HealthCheck))
	mux.HandleFunc("GET /api/pesanan/{id}", handlers.BukaPintuCORS(handlers.GetPesananById))
	mux.HandleFunc("GET /api/pesanan/{id}/struk", handlers.BukaPintuCORS(handlers.CetakStruk))
	mux.HandleFunc("POST /api/login", handlers.BukaPintuCORS(handlers.Login))

	// Route Private (Kasir)
	mux.HandleFunc("GET /api/pesanan", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.GetSemuaPesanan)))
	mux.HandleFunc("GET /api/pesanan/status", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.GetPesananByStatus)))
	mux.HandleFunc("POST /api/pesanan", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.BikinPesananBaru)))
	mux.HandleFunc("PUT /api/pesanan/{id}", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.UpdateStatusPesanan)))
	mux.HandleFunc("GET /api/laporan/neraca", handlers.BukaPintuCORS(handlers.ValidasiJWT(handlers.GetNeraca)))

	// 3. Konfigurasi Server Standar Production (Timeout Mencegah DDoS/Slowloris)
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second, // Maksimal waktu baca request
		WriteTimeout: 10 * time.Second, // Maksimal waktu ngirim response
		IdleTimeout:  15 * time.Second,
	}

	// 4. Graceful Shutdown (Nangkep sinyal Ctrl+C dari terminal)
	go func() {
		fmt.Println("🚀 Server API Ulaundry Berjalan di http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Gagal menjalankan server: %v", err)
		}
	}()

	// Menunggu sinyal interrupt (Ctrl+C)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	fmt.Println("\nMematikan server secara aman (Graceful Shutdown)...")

	// Kasih waktu maksimal 5 detik buat nyelesaiin proses yang lagi jalan (misal ada yang lagi bayar)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server dipaksa mati: %v", err)
	}

	fmt.Println("✅ Server berhasil dimatikan tanpa memotong transaksi.")
}
