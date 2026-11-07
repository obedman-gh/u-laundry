package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

var ErrAuthentication = errors.New("Username atau kata sandi tidak valid.")

type PasswordVerifier interface {
	Compare(hash, password string) bool
}
type TokenIssuer interface {
	Sign(domain.Staff, time.Time) (string, error)
}
type Auth struct {
	uow       repository.UnitOfWork
	runtime   Runtime
	passwords PasswordVerifier
	tokens    TokenIssuer
}

func NewAuth(uow repository.UnitOfWork, rt Runtime, passwords PasswordVerifier, tokens TokenIssuer) *Auth {
	return &Auth{uow, rt, passwords, tokens}
}

type LoginResult struct {
	Token, Name           string
	CredentialFingerprint [32]byte
}

func (s *Auth) Login(ctx context.Context, username, password string) (LoginResult, error) {
	if username == "" || len(username) > 50 || password == "" || len(password) > 72 {
		return LoginResult{}, ErrAuthentication
	}
	var result LoginResult
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		credentials, err := tx.FindCredentials(ctx, username)
		if errors.Is(err, repository.ErrNotFound) {
			s.passwords.Compare("", password) // Equal-cost dummy comparison for unknown usernames.
			return ErrAuthentication
		}
		if err != nil {
			return err
		}
		verified := s.passwords.Compare(credentials.PasswordHash, password)
		if !verified || !credentials.Staff.Active || credentials.Staff.Validate() != nil {
			return ErrAuthentication
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		token, err := s.tokens.Sign(credentials.Staff, at)
		if err != nil {
			return err
		}
		if token == "" {
			return errors.New("Penerbit token tidak menghasilkan token")
		}
		if err := audit(ctx, tx, s.runtime, credentials.Staff.ID, domain.StaffLoggedIn, domain.StaffEntity, credentials.Staff.ID, at); err != nil {
			return err
		}
		result = LoginResult{Token: token, Name: credentials.Staff.Name, CredentialFingerprint: sha256.Sum256([]byte(credentials.PasswordHash))}
		return nil
	})
	if err != nil {
		return LoginResult{}, err
	}
	return result, nil
}

type Pinger interface{ PingContext(context.Context) error }
type Health struct{ db Pinger }

func NewHealth(db Pinger) *Health                 { return &Health{db} }
func (s *Health) Check(ctx context.Context) error { return s.db.PingContext(ctx) }
