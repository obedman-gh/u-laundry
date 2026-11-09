package migrate

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lib/pq"
	"ulaundry/migrations"
)

func TestCatalog(t *testing.T) {
	all, err := catalog(migrations.Files)
	if err != nil || len(all) != 8 {
		t.Fatalf("catalog = %v, %v", all, err)
	}
	for _, files := range []fstest.MapFS{
		{},
		{"000002_gap.up.sql": {Data: []byte("SELECT 1")}},
		{"000001_a.up.sql": {Data: []byte("SELECT 1")}, "000001_b.up.sql": {Data: []byte("SELECT 2")}},
		{"init.sql": {Data: []byte("SELECT 1")}},
		{"000001_empty.up.sql": {}},
	} {
		if _, err := catalog(files); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
}

func TestErrorDoesNotDiscloseData(t *testing.T) {
	err := databaseError("migration", &pq.Error{Code: "23514", Constraint: "keuangan_tipe_allowed", Message: "private customer", Detail: "private row"})
	if strings.Contains(err.Error(), "private") || !strings.Contains(err.Error(), "23514") || !strings.Contains(err.Error(), "keuangan_tipe_allowed") {
		t.Fatal(err)
	}
}
