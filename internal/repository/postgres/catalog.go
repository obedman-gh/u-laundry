package postgres

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

const customerColumns = `id,name,contact,notes,created_at,updated_at`

func scanCustomer(ctx context.Context, row scanner) (domain.Customer, error) {
	var c domain.Customer
	if err := row.Scan(&c.ID, &c.Name, &c.Contact, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return c, dbError(ctx, "read customer", err)
	}
	if err := c.Validate(); err != nil {
		return domain.Customer{}, corrupt("read customer")
	}
	return c, nil
}
func (s *session) GetCustomer(ctx context.Context, id string) (domain.Customer, error) {
	return scanCustomer(ctx, s.tx.QueryRowContext(ctx, `SELECT `+customerColumns+` FROM public.customers WHERE id=$1`, id))
}
func (s *session) SearchCustomers(ctx context.Context, term string, page repository.Page) ([]domain.Customer, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	if !utf8.ValidString(term) || utf8.RuneCountInString(term) > 100 {
		return nil, &domain.Error{Kind: domain.Validation, Code: "invalid_search", Field: "query", Message: "Pencarian maksimal 100 karakter UTF-8."}
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term) + "%"
	return queryAll(ctx, s.tx, "search customers", `SELECT `+customerColumns+` FROM public.customers WHERE name ILIKE $1 ESCAPE '\' OR contact LIKE $1 ESCAPE '\' ORDER BY name,id LIMIT $2 OFFSET $3`, func(row *sql.Rows) (domain.Customer, error) { return scanCustomer(ctx, row) }, pattern, p.Limit, p.Offset)
}
func (s *session) AddCustomer(ctx context.Context, c domain.Customer) error {
	if err := c.Validate(); err != nil {
		return err
	}
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.customers (`+customerColumns+`) VALUES ($1,$2,$3,$4,$5,$6)`, c.ID, c.Name, c.Contact, c.Notes, databaseTime(c.CreatedAt), databaseTime(c.UpdatedAt))
	return dbError(ctx, "add customer", err)
}

const serviceColumns = `id,name,unit,unit_price,turnaround_ns,active,created_at,updated_at`

func scanService(ctx context.Context, row scanner) (domain.Service, error) {
	var v domain.Service
	if err := row.Scan(&v.ID, &v.Name, &v.Unit, &v.UnitPrice, &v.Turnaround, &v.Active, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return v, dbError(ctx, "read service", err)
	}
	if err := v.Validate(); err != nil {
		return domain.Service{}, corrupt("read service")
	}
	return v, nil
}
func (s *session) GetService(ctx context.Context, id string) (domain.Service, error) {
	return scanService(ctx, s.tx.QueryRowContext(ctx, `SELECT `+serviceColumns+` FROM public.services WHERE id=$1`, id))
}
func (s *session) ListActiveServices(ctx context.Context, page repository.Page) ([]domain.Service, error) {
	p, err := page.Normalize()
	if err != nil {
		return nil, err
	}
	return queryAll(ctx, s.tx, "list services", `SELECT `+serviceColumns+` FROM public.services WHERE active=TRUE ORDER BY name,id LIMIT $1 OFFSET $2`, func(row *sql.Rows) (domain.Service, error) { return scanService(ctx, row) }, p.Limit, p.Offset)
}
func (s *session) AddService(ctx context.Context, v domain.Service) error {
	if err := v.Validate(); err != nil {
		return err
	}
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.services (`+serviceColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.Name, v.Unit, v.UnitPrice, int64(v.Turnaround), v.Active, databaseTime(v.CreatedAt), databaseTime(v.UpdatedAt))
	return dbError(ctx, "add service", err)
}

const staffColumns = `id,username,name,role,active,created_at,updated_at`

func scanStaff(ctx context.Context, row scanner) (domain.Staff, error) {
	var v domain.Staff
	if err := row.Scan(&v.ID, &v.Username, &v.Name, &v.Role, &v.Active, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return v, dbError(ctx, "read staff", err)
	}
	if err := v.Validate(); err != nil {
		return domain.Staff{}, corrupt("read staff")
	}
	return v, nil
}
func (s *session) GetStaff(ctx context.Context, id string) (domain.Staff, error) {
	return scanStaff(ctx, s.tx.QueryRowContext(ctx, `SELECT `+staffColumns+` FROM public.staff WHERE id=$1`, id))
}
func (s *session) FindCredentials(ctx context.Context, username string) (repository.Credentials, error) {
	var c repository.Credentials
	v := &c.Staff
	err := s.tx.QueryRowContext(ctx, `SELECT `+staffColumns+`,password_hash FROM public.staff WHERE username=$1`, username).Scan(&v.ID, &v.Username, &v.Name, &v.Role, &v.Active, &v.CreatedAt, &v.UpdatedAt, &c.PasswordHash)
	if err != nil {
		return repository.Credentials{}, dbError(ctx, "find credentials", err)
	}
	if err := v.Validate(); err != nil {
		return repository.Credentials{}, corrupt("find credentials")
	}
	if _, err := bcrypt.Cost([]byte(c.PasswordHash)); err != nil {
		return repository.Credentials{}, corrupt("find credentials")
	}
	return c, nil
}
func (s *session) AddStaff(ctx context.Context, v domain.Staff, passwordHash string) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return &domain.Error{Kind: domain.Validation, Code: "invalid_password_hash", Field: "password_hash", Message: "Hash password tidak valid."}
	}
	_, err := s.tx.ExecContext(ctx, `INSERT INTO public.staff (`+staffColumns+`,password_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.Username, v.Name, v.Role, v.Active, databaseTime(v.CreatedAt), databaseTime(v.UpdatedAt), passwordHash)
	return dbError(ctx, "add staff", err)
}
