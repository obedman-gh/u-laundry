// Package migrations embeds immutable schema migrations in the executable.
package migrations

import "embed"

// Files contains schema only. Demo data must never be embedded here.
//
//go:embed *.up.sql
var Files embed.FS
