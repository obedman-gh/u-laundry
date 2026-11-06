package handler

import "database/sql"

// Database is a transitional SQL boundary, not a domain repository. Keeping the
// existing SQL types avoids changing queries and transaction logic during wiring.
type Database interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
	Begin() (*sql.Tx, error)
	Ping() error
}

type Handler struct {
	db        Database
	jwtSecret string
}

func New(db Database, jwtSecret string) *Handler {
	return &Handler{db: db, jwtSecret: jwtSecret}
}
