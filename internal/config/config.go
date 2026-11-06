// Package config separates startup settings from HTTP and database code.
// Validation and additional deployment settings are added in the next increment.
package config

import (
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Database struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

type HTTP struct {
	Address         string
	StaticDir       string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type Config struct {
	Database      Database
	HTTP          HTTP
	JWTSecret     string
	AllowedOrigin string
}

// Load preserves the legacy .env precedence. A load error is returned separately
// so the executable can warn and continue with the system environment as before.
func Load() (Config, error) {
	err := godotenv.Load()
	return fromEnvironment(os.Getenv), err
}

func fromEnvironment(getenv func(string) string) Config {
	return Config{
		Database: Database{
			Host: getenv("DB_HOST"), Port: getenv("DB_PORT"),
			User: getenv("DB_USER"), Password: getenv("DB_PASS"), Name: getenv("DB_NAME"),
		},
		HTTP: HTTP{
			Address: ":8080", StaticDir: "./frontend",
			ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
			IdleTimeout: 15 * time.Second, ShutdownTimeout: 5 * time.Second,
		},
		JWTSecret: getenv("JWT_SECRET"), AllowedOrigin: getenv("ALLOWED_ORIGIN"),
	}
}
