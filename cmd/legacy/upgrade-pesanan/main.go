package main

// Retired after the replacement passed PostgreSQL migration verification.
import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "Command lama dinonaktifkan. Jalankan go run ./cmd/migrate status lalu go run ./cmd/migrate up; lihat docs/database.md.")
	os.Exit(1)
}
