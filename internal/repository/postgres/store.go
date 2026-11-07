// Package postgres implements use-case repositories with transaction-local DB
// dependencies. Neither raw database errors nor SQL handles cross the contract.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"ulaundry/internal/repository"
)

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

var _ repository.UnitOfWork = (*Store)(nil)

type session struct{ tx *sql.Tx }

var _ repository.Tx = (*session)(nil)

func dbError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%s: %w", operation, ctx.Err())
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: %w", operation, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	}
	kind := repository.Unavailable
	retry := false
	if errors.Is(err, sql.ErrNoRows) {
		kind = repository.NotFound
	} else {
		var pg *pq.Error
		if errors.As(err, &pg) {
			switch string(pg.Code) {
			case "23505":
				kind = repository.Conflict
			case "23502", "23503", "23514", "22001", "22003":
				kind = repository.Constraint
			case "40001", "40P01":
				kind = repository.Conflict
				retry = true
			}
		}
	}
	return &repository.Error{Kind: kind, Operation: operation, Retryable: retry}
}

func corrupt(operation string) error {
	return &repository.Error{Kind: repository.InvalidData, Operation: operation}
}
func stale(operation string) error {
	return &repository.Error{Kind: repository.Conflict, Operation: operation}
}

func (s *Store) View(ctx context.Context, fn func(repository.Readers) error) error {
	return s.run(ctx, true, func(tx repository.Tx) error { return fn(tx) })
}
func (s *Store) WithinTransaction(ctx context.Context, fn func(repository.Tx) error) error {
	return s.run(ctx, false, fn)
}

func (s *Store) run(ctx context.Context, readOnly bool, fn func(repository.Tx) error) (result error) {
	isolation := sql.LevelReadCommitted
	if readOnly {
		isolation = sql.LevelRepeatableRead
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly, Isolation: isolation})
	if err != nil {
		return dbError(ctx, "begin transaction", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			result = errors.Join(result, dbError(context.Background(), "rollback transaction", err))
		}
	}()
	if err := fn(&session{tx: tx}); err != nil {
		return err
	}
	return dbError(ctx, "commit transaction", tx.Commit())
}

// queryAll ensures scan failures, iterator failures, and close failures are never
// ignored. It always returns a non-nil empty slice for an empty result set.
func queryAll[T any](ctx context.Context, tx *sql.Tx, operation, query string, scan func(*sql.Rows) (T, error), args ...any) (items []T, result error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, dbError(ctx, operation, err)
	}
	defer func() { result = errors.Join(result, dbError(ctx, operation+" close", rows.Close())) }()
	items = make([]T, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(ctx, operation, err)
	}
	return items, nil
}

type scanner interface{ Scan(...any) error }

// PostgreSQL timestamps have microsecond precision. Floor at the adapter edge
// so a round-up on insert cannot make the next event appear to predate creation.
func databaseTime(at time.Time) time.Time { return at.UTC().Truncate(time.Microsecond) }
