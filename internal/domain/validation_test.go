package domain_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ulaundry/internal/domain"
)

func TestCustomerValidationAndContacts(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"", ""}, {"  ", ""}, {"0812-3456-7890", "+6281234567890"},
		{"62 812 3456 7890", "+6281234567890"}, {"+62 (812) 3456-7890", "+6281234567890"},
		{"+14155552671", "+14155552671"}, {"+12345678", "+12345678"}, {"+123456789012345", "+123456789012345"},
	} {
		got, err := domain.NormalizeContact(tc.raw)
		if err != nil || got != tc.want {
			t.Fatalf("normalization got=%q err=%v", got, err)
		}
	}
	for _, raw := range []string{"abc", "12345678", "+1234567", "+1234567890123456", "+0123456789", "08+12345678", "+62８12345678", "08123456789 ext 1", "( )", "0812345\n6789", "+62<script>"} {
		_, err := domain.NormalizeContact(raw)
		requireError(t, err, domain.Validation, "invalid_value")
	}
	c, err := domain.NewCustomer("c-1", "  Éka 李  ", "", "  Baris satu\r\nBaris dua  ", now)
	if err != nil || c.Name != "Éka 李" || c.Notes != "Baris satu\nBaris dua" {
		t.Fatalf("customer normalization: %+v %v", c, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.Customer)
	}{
		{"empty_name", func(c *domain.Customer) { c.Name = "" }},
		{"name_too_long", func(c *domain.Customer) { c.Name = strings.Repeat("李", 101) }},
		{"name_control", func(c *domain.Customer) { c.Name = "Nama\nBaru" }},
		{"name_invisible", func(c *domain.Customer) { c.Name = "Nama\u200b" }},
		{"name_invalid_utf8", func(c *domain.Customer) { c.Name = string([]byte{0xff}) }},
		{"notes_limit", func(c *domain.Customer) { c.Notes = strings.Repeat("x", 1001) }},
		{"noncanonical_contact", func(c *domain.Customer) { c.Contact = "081234567890" }},
		{"bad_id", func(c *domain.Customer) { c.ID = "invalid id" }},
		{"missing_time", func(c *domain.Customer) { c.CreatedAt = time.Time{} }},
		{"reversed_time", func(c *domain.Customer) { c.UpdatedAt = now.Add(-time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := c
			tc.change(&invalid)
			requireError(t, invalid.Validate(), domain.Validation, "invalid_value")
		})
	}
	if _, err := domain.NewCustomer("c-1", strings.Repeat("李", 100), "", strings.Repeat("x", 1000), now); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.NewCustomer("c-1", "   ", "", "", now); err == nil {
		t.Fatal("blank customer name accepted")
	}
}

func TestServiceAndStaffValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.Service)
	}{
		{"service_empty_name", func(s *domain.Service) { s.Name = "" }},
		{"service_unknown_unit", func(s *domain.Service) { s.Unit = "litre" }},
		{"service_negative_price", func(s *domain.Service) { s.UnitPrice = -1 }},
		{"service_zero_duration", func(s *domain.Service) { s.Turnaround = 0 }},
		{"service_negative_duration", func(s *domain.Service) { s.Turnaround = -time.Hour }},
		{"service_bad_timestamp", func(s *domain.Service) { s.UpdatedAt = time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := service()
			tc.change(&s)
			requireError(t, s.Validate(), domain.Validation, "invalid_value")
		})
	}
	for _, role := range []domain.Role{domain.Owner, domain.Admin, domain.Cashier} {
		if err := staff(role).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.Staff)
	}{
		{"staff_unknown_role", func(s *domain.Staff) { s.Role = "superuser" }},
		{"staff_blank_name", func(s *domain.Staff) { s.Name = "" }},
		{"staff_short_username", func(s *domain.Staff) { s.Username = "ab" }},
		{"staff_long_username", func(s *domain.Staff) { s.Username = strings.Repeat("a", 51) }},
		{"staff_username_spaces", func(s *domain.Staff) { s.Username = "a b" }},
		{"staff_username_nonascii", func(s *domain.Staff) { s.Username = "stáff" }},
		{"staff_bad_time", func(s *domain.Staff) { s.UpdatedAt = now.Add(-time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := staff(domain.Cashier)
			tc.change(&s)
			requireError(t, s.Validate(), domain.Validation, "invalid_value")
		})
	}
	// Keep usernames case-sensitive, matching existing unique/login semantics.
	s := staff(domain.Cashier)
	s.Username = "Staf.Mixed"
	if err := s.Validate(); err != nil || s.Username != "Staf.Mixed" {
		t.Fatal("username changed")
	}
}

func TestLedgerIsDerivedFromPayment(t *testing.T) {
	p := payment("p1", 12000, now)
	entry, err := domain.NewPaymentLedger("ledger-1", p)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Amount != p.Amount || entry.PaymentID != p.ID || entry.OrderID != p.OrderID || entry.Type != domain.Debit || entry.RecordedBy != p.RecordedBy || !entry.OccurredAt.Equal(p.ReceivedAt) {
		t.Fatal("ledger differs from payment")
	}
	for _, method := range []domain.PaymentMethod{domain.Cash, domain.BankTransfer, domain.QRIS} {
		p.Method = method
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.LedgerEntry)
	}{
		{"invalid_type", func(e *domain.LedgerEntry) { e.Type = "debit" }},
		{"negative_amount", func(e *domain.LedgerEntry) { e.Amount = -1 }},
		{"zero_amount", func(e *domain.LedgerEntry) { e.Amount = 0 }},
		{"missing_payment_order", func(e *domain.LedgerEntry) { e.OrderID = "" }},
		{"payment_outflow_not_supported", func(e *domain.LedgerEntry) { e.Type = domain.Credit }},
		{"missing_actor", func(e *domain.LedgerEntry) { e.RecordedBy = "" }},
		{"missing_description", func(e *domain.LedgerEntry) { e.Description = "" }},
		{"missing_time", func(e *domain.LedgerEntry) { e.OccurredAt = time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := entry
			tc.change(&e)
			requireError(t, e.Validate(), domain.Validation, "invalid_value")
		})
	}
	// An actual operating expense has no order/payment reference.
	expense := domain.LedgerEntry{ID: "expense-1", Type: domain.Credit, Amount: 45000, Description: "Pembelian sabun", RecordedBy: "staff-1", OccurredAt: now}
	if err := expense.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Amount = -1
	_, err = domain.NewPaymentLedger("ledger-1", p)
	requireError(t, err, domain.Validation, "invalid_value")
}

func TestAuditAndTypedErrors(t *testing.T) {
	for _, pair := range []struct {
		action domain.AuditAction
		entity domain.AuditEntity
	}{
		{domain.OrderCreated, domain.OrderEntity}, {domain.OrderStatusChanged, domain.OrderEntity}, {domain.DiscountApproved, domain.OrderEntity},
		{domain.PaymentRecorded, domain.PaymentEntity}, {domain.CustomerUpdated, domain.CustomerEntity}, {domain.ServiceUpdated, domain.ServiceEntity}, {domain.StaffUpdated, domain.StaffEntity},
	} {
		e := domain.AuditEvent{ID: "event-1", ActorID: "staff-1", EntityID: "object-1", Action: pair.action, Entity: pair.entity, OccurredAt: now}
		if err := e.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	base := domain.AuditEvent{ID: "event-1", ActorID: "staff-1", EntityID: "order-1", Action: domain.OrderCreated, Entity: domain.OrderEntity, OccurredAt: now}
	for _, change := range []func(*domain.AuditEvent){
		func(e *domain.AuditEvent) { e.Action = "unknown" },
		func(e *domain.AuditEvent) { e.Entity = domain.StaffEntity },
		func(e *domain.AuditEvent) { e.ActorID = "" },
		func(e *domain.AuditEvent) { e.OccurredAt = time.Time{} },
	} {
		e := base
		change(&e)
		requireError(t, e.Validate(), domain.Validation, "invalid_value")
	}
	_, err := domain.NormalizeContact("private-person@example.invalid")
	err = fmt.Errorf("use case: %w", err)
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Field != "contact" || strings.Contains(err.Error(), "private-person") {
		t.Fatal("error type or privacy")
	}
}
