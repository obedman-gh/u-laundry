package platform

import (
	"database/sql"
	"fmt"

	"ulaundry/internal/config"

	_ "github.com/lib/pq"
)

// OpenDatabase preserves the legacy DSN and ping policy. Context deadlines,
// SSL configuration and pool settings belong to the configuration increment.
func OpenDatabase(settings config.Database) (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		settings.Host, settings.Port, settings.User, settings.Password, settings.Name)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka database: %w", err)
	}
	if err := db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("database belum siap: %w; gagal menutup koneksi: %w", err, closeErr)
		}
		return nil, fmt.Errorf("database belum siap: %w", err)
	}
	return db, nil
}
