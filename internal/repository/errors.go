package repository

import "errors"

type ErrorKind string

const (
	NotFound    ErrorKind = "not_found"
	Conflict    ErrorKind = "conflict"
	Constraint  ErrorKind = "constraint"
	Unavailable ErrorKind = "unavailable"
	InvalidData ErrorKind = "invalid_data"
)

var (
	ErrNotFound    = errors.New("Data tidak ditemukan.")
	ErrConflict    = errors.New("Data bertentangan dengan keadaan saat ini.")
	ErrConstraint  = errors.New("Data melanggar aturan penyimpanan.")
	ErrUnavailable = errors.New("Penyimpanan tidak tersedia.")
	ErrInvalidData = errors.New("Data tersimpan tidak sesuai aturan domain.")
)

// Error intentionally exposes no driver cause or database details, even via
// errors.As/Unwrap. Kind/Operation/Retryable suffice for later service mapping.
type Error struct {
	Kind      ErrorKind
	Operation string
	Retryable bool
}

func (e *Error) Error() string { return e.Operation + ": " + e.Unwrap().Error() }
func (e *Error) Unwrap() error {
	switch e.Kind {
	case NotFound:
		return ErrNotFound
	case Conflict:
		return ErrConflict
	case Constraint:
		return ErrConstraint
	case InvalidData:
		return ErrInvalidData
	default:
		return ErrUnavailable
	}
}
