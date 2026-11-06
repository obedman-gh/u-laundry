package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ulaundry/models"
)

// The standard SQL connector exercises the injected SQL handle without network,
// .env, global driver registration, real customer records or a PostgreSQL server.
type orderConnector struct {
	name     string
	queryErr error
}

func (c orderConnector) Connect(context.Context) (driver.Conn, error) {
	return &orderConnection{fixture: c}, nil
}
func (c orderConnector) Driver() driver.Driver { return fixtureDriver{} }

type fixtureDriver struct{}

func (fixtureDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use the isolated connector")
}

type orderConnection struct{ fixture orderConnector }

func (*orderConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not used by list handler")
}
func (*orderConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("transaction not used by list handler")
}
func (*orderConnection) Close() error { return nil }
func (c *orderConnection) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	if c.fixture.queryErr != nil {
		return nil, c.fixture.queryErr
	}
	return &orderRows{name: c.fixture.name}, nil
}

type orderRows struct {
	name string
	done bool
}

func (*orderRows) Columns() []string {
	return []string{"id_pesanan", "nama_customer", "layanan", "total_biaya", "status", "waktu_pesan", "waktu_perkiraan_selesai", "waktu_selesai_real"}
}
func (*orderRows) Close() error { return nil }
func (r *orderRows) Next(values []driver.Value) error {
	if r.done || r.name == "" {
		return io.EOF
	}
	r.done = true
	copy(values, []driver.Value{"DEMO-01", r.name, "Cuci", int64(12000), "Diterima", "2026-11-07 08:00:00", nil, nil})
	return nil
}

func fixtureDB(t *testing.T, connector orderConnector) *sql.DB {
	t.Helper()
	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestGetSemuaPesanan(t *testing.T) {
	// Construct both handlers first: one must not replace another's database.
	first := New(fixtureDB(t, orderConnector{name: "Pelanggan Demo A"}), "fixture")
	second := New(fixtureDB(t, orderConnector{name: "Pelanggan Demo B"}), "fixture")
	for _, tc := range []struct {
		name string
		h    *Handler
	}{{"Pelanggan Demo A", first}, {"Pelanggan Demo B", second}} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.h.GetSemuaPesanan(rr, httptest.NewRequest("GET", "/api/pesanan", nil))
			if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected response status/content type: %d", rr.Code)
			}
			var orders []models.Pesanan
			if err := json.Unmarshal(rr.Body.Bytes(), &orders); err != nil {
				t.Fatal(err)
			}
			if len(orders) != 1 || orders[0].NamaCustomer != tc.name || orders[0].IDPesanan != "DEMO-01" || orders[0].TotalBiaya != 12000 || orders[0].WaktuPerkiraanSelesai != nil {
				t.Fatal("injected fixture or response shape was not preserved")
			}
		})
	}
}

func TestGetSemuaPesananQueryFailure(t *testing.T) {
	h := New(fixtureDB(t, orderConnector{queryErr: errors.New("synthetic query failure")}), "fixture")
	rr := httptest.NewRecorder()
	h.GetSemuaPesanan(rr, httptest.NewRequest("GET", "/api/pesanan", nil))
	if rr.Code != http.StatusInternalServerError || rr.Body.String() != "Gagal query ke database\n" {
		t.Fatal("legacy query failure response changed")
	}
}
