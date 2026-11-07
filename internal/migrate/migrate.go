// Package migrate runs the small, forward-only ULaundry migration protocol.
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"time"

	"github.com/lib/pq"
	"ulaundry/migrations"
)

const lockKey int64 = 7643296100401

type migration struct {
	version             int
	name, checksum, sql string
}

type State struct {
	Version   int
	Name      string
	AppliedAt *time.Time
}

var filename = regexp.MustCompile(`^([0-9]{6})_[a-z0-9_]+\.up\.sql$`)

func catalog(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, err
	}
	var all []migration
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, errors.New("migration catalog contains a directory")
		}
		parts := filename.FindStringSubmatch(entry.Name())
		if parts == nil {
			return nil, fmt.Errorf("invalid migration filename: %s", entry.Name())
		}
		version, err := strconv.Atoi(parts[1])
		if err != nil || version != len(all)+1 {
			return nil, errors.New("migration versions must be contiguous from 000001")
		}
		body, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return nil, err
		}
		if len(body) == 0 {
			return nil, errors.New("empty migration")
		}
		all = append(all, migration{version, entry.Name(), fmt.Sprintf("%x", sha256.Sum256(body)), string(body)})
	}
	if len(all) == 0 {
		return nil, errors.New("empty migration catalog")
	}
	return all, nil
}

// databaseError deliberately omits PostgreSQL Detail/Message, which can contain
// customer values. SQLSTATE and the constraint identify the repair needed.
func databaseError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pg *pq.Error
	if errors.As(err, &pg) {
		return fmt.Errorf("%s: SQLSTATE %s, constraint %s", operation, pg.Code, pg.Constraint)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return fmt.Errorf("%s gagal (detail koneksi/data tidak ditampilkan)", operation)
}

func rollback(tx *sql.Tx, result *error) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		*result = errors.Join(*result, databaseError("rollback", err))
	}
}

func history(ctx context.Context, tx *sql.Tx, all []migration) (states []State, result error) {
	states = make([]State, len(all))
	for i, m := range all {
		states[i] = State{Version: m.version, Name: m.name}
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return nil, databaseError("read migration table", err)
	}
	if !exists {
		return states, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT version, name, checksum, applied_at FROM public.schema_migrations ORDER BY version`)
	if err != nil {
		return nil, databaseError("read migration history", err)
	}
	defer func() { result = errors.Join(result, databaseError("close migration history", rows.Close())) }()
	i := 0
	for rows.Next() {
		var version int
		var name, checksum string
		var appliedAt time.Time
		if err := rows.Scan(&version, &name, &checksum, &appliedAt); err != nil {
			return nil, databaseError("scan migration history", err)
		}
		if i >= len(all) || version != all[i].version || name != all[i].name || checksum != all[i].checksum {
			return nil, errors.New("migration history berbeda: versi tidak dikenal, urutan berlubang atau checksum berubah; gunakan executable yang sesuai, jangan edit history")
		}
		states[i].AppliedAt = &appliedAt
		i++
	}
	if err := rows.Err(); err != nil {
		return nil, databaseError("iterate migration history", err)
	}
	return states, nil
}

// Status is read-only, including on a database with no history table.
func Status(ctx context.Context, db *sql.DB) (states []State, result error) {
	all, err := catalog(migrations.Files)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, databaseError("begin status", err)
	}
	defer rollback(tx, &result)
	states, err = history(ctx, tx, all)
	if err != nil {
		return nil, err
	}
	return states, databaseError("commit status", tx.Commit())
}

// Up applies all pending versions and their history in ONE atomic transaction.
// Transaction-scoped advisory lock is released on commit/rollback/disconnect.
// Concurrent runners fail fast. No baseline stamp/force/down bypass is provided.
func Up(ctx context.Context, db *sql.DB) (result error) {
	all, err := catalog(migrations.Files)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return databaseError("begin migration", err)
	}
	defer rollback(tx, &result)
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockKey).Scan(&locked); err != nil {
		return databaseError("migration lock", err)
	}
	if !locked {
		return errors.New("migration lain sedang berjalan; ulangi setelah selesai")
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path = pg_catalog, public; SET LOCAL lock_timeout = '5s'`); err != nil {
		return databaseError("migration settings", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
        version INTEGER PRIMARY KEY CHECK (version > 0),
        name TEXT NOT NULL UNIQUE,
        checksum TEXT NOT NULL CHECK (length(checksum) = 64),
        applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
    )`); err != nil {
		return databaseError("create migration history", err)
	}
	states, err := history(ctx, tx, all)
	if err != nil {
		return err
	}
	for i, m := range all {
		if states[i].AppliedAt != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			return databaseError("migration "+m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`, m.version, m.name, m.checksum); err != nil {
			return databaseError("record migration "+m.name, err)
		}
	}
	return databaseError("commit migration (jika koneksi putus, periksa status sebelum retry)", tx.Commit())
}
