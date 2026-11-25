package domain_test

import (
	"strings"
	"testing"
	"ulaundry/internal/domain"
)

func TestAuditMetadataAndCorrelation(t *testing.T) {
	negative := domain.Money(-1)
	for _, m := range []domain.AuditMetadata{{Version: -1}, {Status: "private text"}, {Role: "password"}, {UnitPrice: &negative}} {
		if m.Validate() == nil {
			t.Fatal("unsafe metadata accepted", m)
		}
	}
	zero := domain.Money(0)
	active := false
	if err := (domain.AuditMetadata{Version: 1, Status: domain.Processing, Role: domain.Cashier, Active: &active, UnitPrice: &zero}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "bad/request", "\nsecret", strings.Repeat("x", 101), "akun dengan spasi"} {
		if domain.ValidAuditRequestID(id) {
			t.Fatal("bad request id", id)
		}
	}
	if !domain.ValidAuditRequestID("server-request_1.test") || !domain.CredentialsVerified.Valid() || !domain.ShiftEntity.Valid() || !domain.RefundEntity.Valid() {
		t.Fatal("valid identifiers rejected")
	}
}
