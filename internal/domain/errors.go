// Package domain contains laundry business rules, independent of transport,
// persistence, configuration, and the system clock. It is not an API DTO.
package domain

// ErrorKind and Error.Code are stable identifiers for a future boundary mapper.
// No HTTP status codes or database errors belong in this package.
type ErrorKind string

const (
	Validation ErrorKind = "validation"
	Conflict   ErrorKind = "conflict"
	Forbidden  ErrorKind = "forbidden"
	NotFound   ErrorKind = "not_found"
)

// Error contains a safe static message, never the rejected value or personal data.
// Callers can inspect wrapped errors using errors.As(err, *domain.Error).
type Error struct {
	Kind    ErrorKind
	Code    string
	Field   string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Field + ": " + e.Message }

func invalid(field, message string) error {
	return &Error{Kind: Validation, Code: "invalid_value", Field: field, Message: message}
}

func conflict(code, field, message string) error {
	return &Error{Kind: Conflict, Code: code, Field: field, Message: message}
}
