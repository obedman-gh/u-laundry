package domain

import (
	"strings"
	"time"
)

type Customer struct {
	ID, Name, Contact, Notes string
	Address                  string
	CreatedAt, UpdatedAt     time.Time
}

func NewCustomer(id, name, contact, notes string, at time.Time) (Customer, error) {
	number, err := NormalizeContact(contact)
	if err != nil {
		return Customer{}, err
	}
	c := Customer{ID: id, Name: strings.TrimSpace(name), Contact: number, Notes: cleanNotes(notes), CreatedAt: at, UpdatedAt: at}
	if err := c.Validate(); err != nil {
		return Customer{}, err
	}
	return c, nil
}

func (c Customer) Validate() error {
	if err := identifier("customer.id", c.ID); err != nil {
		return err
	}
	if err := text("customer.name", c.Name, 100, true, false); err != nil {
		return err
	}
	normalized, err := NormalizeContact(c.Contact)
	if err != nil {
		return err
	}
	if normalized != c.Contact {
		return invalid("customer.contact", "Nomor kontak harus dinormalisasi.")
	}
	if err := text("customer.notes", c.Notes, 1000, false, true); err != nil {
		return err
	}
	if err := text("customer.address", c.Address, 500, false, true); err != nil {
		return err
	}
	return timestamps(c.CreatedAt, c.UpdatedAt)
}

func (c Customer) Edit(name, contact, address, notes string, at time.Time) (Customer, error) {
	number, err := NormalizeContact(contact)
	if err != nil {
		return Customer{}, err
	}
	c.Name, c.Contact, c.Address, c.Notes, c.UpdatedAt = strings.TrimSpace(name), number, cleanNotes(address), cleanNotes(notes), at
	if err := c.Validate(); err != nil {
		return Customer{}, err
	}
	return c, nil
}

func CustomerSearch(term string) (string, error) {
	term = strings.TrimSpace(term)
	if err := text("query", term, 100, false, false); err != nil {
		return "", err
	}
	// Normalize a complete phone number so local and +62 searches agree.
	if number, err := NormalizeContact(term); err == nil && number != "" {
		return number, nil
	}
	return term, nil
}

type Unit string

const (
	PerKilogram Unit = "kg"
	PerPiece    Unit = "piece"
)

type Service struct {
	ID, Name             string
	Unit                 Unit
	UnitPrice            Money
	Turnaround           time.Duration
	Active               bool
	DisplayOrder         int
	CreatedAt, UpdatedAt time.Time
}

func (s Service) Validate() error {
	if err := identifier("service.id", s.ID); err != nil {
		return err
	}
	if err := text("service.name", s.Name, 100, true, false); err != nil {
		return err
	}
	if s.Unit != PerKilogram && s.Unit != PerPiece {
		return invalid("service.unit", "Satuan layanan harus kg atau piece.")
	}
	if err := s.UnitPrice.Validate("service.unit_price"); err != nil {
		return err
	}
	if s.Turnaround <= 0 {
		return invalid("service.turnaround", "Durasi layanan harus positif.")
	}
	if s.DisplayOrder < 0 || s.DisplayOrder > 1000000 {
		return invalid("service.display_order", "Urutan tampilan harus 0–1000000.")
	}
	return timestamps(s.CreatedAt, s.UpdatedAt)
}

type Role string

const (
	Owner   Role = "owner"
	Admin   Role = "admin"
	Cashier Role = "cashier"
)

// Staff is the staff/user identity. Credentials and sessions live outside domain.
type Staff struct {
	ID, Username, Name   string
	Role                 Role
	Active               bool
	CreatedAt, UpdatedAt time.Time
}

func (s Staff) Validate() error {
	if err := identifier("staff.id", s.ID); err != nil {
		return err
	}
	if len(s.Username) < 3 || len(s.Username) > 50 {
		return invalid("staff.username", "Username harus 3–50 karakter.")
	}
	if err := identifier("staff.username", s.Username); err != nil {
		return err
	}
	if err := text("staff.name", s.Name, 100, true, false); err != nil {
		return err
	}
	if s.Role != Owner && s.Role != Admin && s.Role != Cashier {
		return invalid("staff.role", "Peran staf tidak valid.")
	}
	return timestamps(s.CreatedAt, s.UpdatedAt)
}

func (s Staff) requireActive() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if !s.Active {
		return &Error{Kind: Forbidden, Code: "staff_inactive", Field: "staff", Message: "Staf tidak aktif."}
	}
	return nil
}
