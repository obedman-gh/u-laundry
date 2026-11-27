package service

import (
	"context"
	"testing"
	"ulaundry/internal/domain"
)

func TestOperationsAuthorizationUsesCurrentActiveOwner(t *testing.T) {
	for _, name := range []string{"owner", "admin", "cashier", "inactive", "missing"} {
		role := domain.Role(name)
		if name == "inactive" {
			role = domain.Owner
		}
		if name == "missing" {
			role = ""
		}
		m, _, _ := staffFixture(role)
		if name == "inactive" {
			v := m.rows["actor"]
			v.Staff.Active = false
			m.rows["actor"] = v
		}
		s := NewOperations(WithAccessAudit(m, m))
		err := s.Authorize(context.Background(), "actor")
		if (err == nil) != (name == "owner") {
			t.Fatal("metrics authorization", name, err)
		}
		if name != "owner" && len(m.denials) != 1 {
			t.Fatal("missing access audit", name)
		}
	}
}
