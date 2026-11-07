package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "Command lama dinonaktifkan. Skema: go run ./cmd/migrate up. Data demo terpisah: scripts/demo.sql; lihat docs/database.md.")
	os.Exit(1)
}
