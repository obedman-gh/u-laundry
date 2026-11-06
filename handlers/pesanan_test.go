package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"ulaundry/config"

	"github.com/joho/godotenv"
)

// init() ini bakal jalan duluan buat nyiapin koneksi DB sebelum test dimulai
func init() {
	// Pastikan kita naik satu folder ke root untuk cari .env
	os.Chdir("..")
	godotenv.Load()
	config.ConnectDB()
}

// Integration Test: Mengecek Endpoint GET Semua Pesanan
func TestGetSemuaPesanan(t *testing.T) {
	// 1. Bikin HTTP Request bohongan (simulasi dari client)
	req, err := http.NewRequest("GET", "/api/pesanan/ORD-006", nil)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Bikin penampung response (Recorder)
	rr := httptest.NewRecorder()

	// 3. Panggil fungsi handler-nya secara langsung
	handler := http.HandlerFunc(GetSemuaPesanan)
	handler.ServeHTTP(rr, req)

	// 4. Validasi 1: Apakah HTTP Status Code-nya 200 OK?
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Error Status: dapet %v, harusnya %v", status, http.StatusOK)
	}

	// 5. Validasi 2: Apakah balikan formatnya benar-benar JSON?
	expectedContentType := "application/json"
	if contentType := rr.Header().Get("Content-Type"); contentType != expectedContentType {
		t.Errorf("Error Content-Type: dapet %v, harusnya %v", contentType, expectedContentType)
	}

	// Tampilkan log sukses
	t.Logf("Response Body: %v", rr.Body.String())
}
