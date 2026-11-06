package config

import (
	"database/sql"
	"log"

	appconfig "ulaundry/internal/config"
	"ulaundry/internal/platform"
)

var DB *sql.DB

// ConnectDB is a temporary adapter for the legacy setup utilities only.
// The application uses constructor injection instead of this global handle.
func ConnectDB() {
	settings, err := appconfig.Load()
	if err != nil {
		log.Println("Warning: File .env tidak ditemukan, menggunakan environment system")
	}
	DB, err = platform.OpenDatabase(settings.Database)
	if err != nil {
		log.Fatal("Database tidak tersedia; periksa konfigurasi dan koneksi PostgreSQL")
	}
}
