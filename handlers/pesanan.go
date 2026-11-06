package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	// Ganti "ulaundry" dengan nama module lu di go.mod (kalo beda)
	"ulaundry/config"
	"ulaundry/models"
)

// Pindahin fungsi getSemuaPesanan dari main.go kemaren
func GetSemuaPesanan(w http.ResponseWriter, r *http.Request) {
	// Panggil koneksi DB dari package config
	// Ubah query SELECT lu di GetSemuaPesanan (dan fungsi filter status) jadi kayak gini:
	query := `
	SELECT 
		id_pesanan, nama_customer, layanan, total_biaya, status,
		TO_CHAR(waktu_pesan, 'YYYY-MM-DD HH24:MI:SS'),
		TO_CHAR(waktu_perkiraan_selesai, 'YYYY-MM-DD HH24:MI:SS'),
		TO_CHAR(waktu_selesai_real, 'YYYY-MM-DD HH24:MI:SS')
	FROM pesanan
`
	rows, err := config.DB.Query(query)

	if err != nil {
		http.Error(w, "Gagal query ke database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var daftarPesanan []models.Pesanan
	for rows.Next() {
		var p models.Pesanan
		// ... lalu di dalam perulangan for rows.Next() ..., update Scan-nya:
		if err := rows.Scan(&p.IDPesanan, &p.NamaCustomer, &p.Layanan, &p.TotalBiaya, &p.Status, &p.WaktuPesan, &p.WaktuPerkiraanSelesai, &p.WaktuSelesaiReal); err != nil {
			// ...
			http.Error(w, "Gagal baca data", http.StatusInternalServerError)
			return
		}
		daftarPesanan = append(daftarPesanan, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(daftarPesanan)
}

// Pindahin fungsi bikinPesananBaru
func BikinPesananBaru(w http.ResponseWriter, r *http.Request) {
	var p models.Pesanan
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Format JSON salah", http.StatusBadRequest)
		return
	}

	// Cari bagian query INSERT di BikinPesananBaru, lalu ganti jadi:
	query := `INSERT INTO pesanan (id_pesanan, nama_customer, layanan, total_biaya, waktu_perkiraan_selesai) 
    	      VALUES ($1, $2, $3, $4, $5)`
	_, err := config.DB.Exec(query, p.IDPesanan, p.NamaCustomer, p.Layanan, p.TotalBiaya, p.WaktuPerkiraanSelesai)
	if err != nil {
		http.Error(w, "Gagal simpan ke database", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"pesan": "Pesanan " + p.IDPesanan + " berhasil disimpan!"})
}

// --- TAMBAHIN KODE INI DI BAWAH ---

// Fungsi buat UPDATE status pesanan (PUT Request)
func UpdateStatusPesanan(w http.ResponseWriter, r *http.Request) {
	// Ambil ID dari URL (misal: /api/pesanan/ORD-001)
	idPesanan := r.PathValue("id")

	// Kita baca status baru dari Body JSON
	var input struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Format JSON salah", http.StatusBadRequest)
		return
	}

	// Eksekusi UPDATE ke Database
	// Cari bagian ini di UpdateStatusPesanan lu, tambahin logika waktu selesainya
	query := `UPDATE pesanan SET status = $1 WHERE id_pesanan = $2`

	// --- TAMBAHIN BLOK INI ---
	if input.Status == "Selesai & Siap Diambil" {
		query = `UPDATE pesanan SET status = $1, waktu_selesai_real = CURRENT_TIMESTAMP WHERE id_pesanan = $2`
	}
	// -------------------------

	result, err := config.DB.Exec(query, input.Status, idPesanan)
	if err != nil {
		http.Error(w, "Gagal update database", http.StatusInternalServerError)
		return
	}

	// Ngecek apakah ID-nya beneran ada (RowsAffected)
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Pesanan tidak ditemukan", http.StatusNotFound)
		return
	}

	// --- LOGIKA KASIR OTOMATIS ---
	if input.Status == "Sudah Lunas" {
		var nominal int
		// Tarik total biaya pesanan ini
		err := config.DB.QueryRow("SELECT total_biaya FROM pesanan WHERE id_pesanan = $1", idPesanan).Scan(&nominal)
		if err == nil {
			// Catat ke buku kas sebagai DEBIT (Pemasukan)
			config.DB.Exec("INSERT INTO keuangan (tipe, keterangan, nominal) VALUES ('DEBIT', $1, $2)", "Pembayaran "+idPesanan, nominal)
		}
	}
	// ------------------------------

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"pesan": "Status pesanan " + idPesanan + " berhasil diubah jadi " + input.Status,
	})
}

// Fungsi buat Filter pesanan berdasarkan status (GET Request)
func GetPesananByStatus(w http.ResponseWriter, r *http.Request) {
	// Ambil parameter status dari URL query (misal: /api/pesanan/status?q=Selesai)
	statusQuery := r.URL.Query().Get("q")

	if statusQuery == "" {
		http.Error(w, "Query status (q) tidak boleh kosong", http.StatusBadRequest)
		return
	}

	// Panggil database pake filter WHERE status = $1
	query := `
	SELECT 
		id_pesanan, nama_customer, layanan, total_biaya, status,
		TO_CHAR(waktu_pesan, 'YYYY-MM-DD HH24:MI:SS'),
		TO_CHAR(waktu_perkiraan_selesai, 'YYYY-MM-DD HH24:MI:SS'),
		TO_CHAR(waktu_selesai_real, 'YYYY-MM-DD HH24:MI:SS')
	FROM pesanan
`
	rows, err := config.DB.Query(query)

	if err != nil {
		http.Error(w, "Gagal query ke database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var daftarPesanan []models.Pesanan
	for rows.Next() {
		var p models.Pesanan
		if err := rows.Scan(&p.IDPesanan, &p.NamaCustomer, &p.Layanan, &p.TotalBiaya, &p.Status, &p.WaktuPesan, &p.WaktuPerkiraanSelesai, &p.WaktuSelesaiReal); err != nil {
			http.Error(w, "Gagal baca data", http.StatusInternalServerError)
			return
		}
		daftarPesanan = append(daftarPesanan, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(daftarPesanan)
}

// Fungsi buat Cetak Struk (Ngeluarin HTML khusus printer kasir)
func CetakStruk(w http.ResponseWriter, r *http.Request) {
	idPesanan := r.PathValue("id")

	// Cari pesanan di database
	var p models.Pesanan
	query := "SELECT id_pesanan, nama_customer, layanan, total_biaya, status, TO_CHAR(waktu_pesan, 'YYYY-MM-DD HH24:MI:SS'), TO_CHAR(waktu_perkiraan_selesai, 'YYYY-MM-DD HH24:MI:SS'), TO_CHAR(waktu_selesai_real, 'YYYY-MM-DD HH24:MI:SS') FROM pesanan WHERE id_pesanan = $1"

	err := config.DB.QueryRow(query, idPesanan).Scan(&p.IDPesanan, &p.NamaCustomer, &p.Layanan, &p.TotalBiaya, &p.Status, &p.WaktuPesan, &p.WaktuPerkiraanSelesai, &p.WaktuSelesaiReal)

	if err != nil {
		http.Error(w, "Pesanan tidak ditemukan", http.StatusNotFound)
		return
	}

	// --- LOGIKA AMAN UNTUK POINTER ---
	// Cek apakah waktu selesai masih kosong (nil) di database
	strWaktuSelesai := "-"
	if p.WaktuSelesaiReal != nil {
		strWaktuSelesai = *p.WaktuSelesaiReal
	}
	// ---------------------------------

	// Bikin desain struk HTML
	htmlStruk := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Struk ` + p.IDPesanan + `</title>
		<style>
			body { font-family: monospace; width: 300px; margin: 0 auto; padding: 10px; color: #000; }
			.center { text-align: center; }
			.line { border-bottom: 1px dashed #000; margin: 10px 0; }
			h2 { margin: 5px 0; }
			@media print { body { width: 100%; margin: 0; } }
		</style>
	</head>
	<body onload="window.print()">
		<div class="center">
			<h2>ULAUNDRY</h2>
			<p>Perumahan Viola<br>WA: 0812-3456-7890</p>
		</div>
		<div class="line"></div>
		<p>
			ID Order : ` + p.IDPesanan + `<br>
			Nama     : ` + p.NamaCustomer + `<br>
			Layanan  : ` + p.Layanan + `<br>
			Mulai    : ` + p.WaktuPesan + `<br>
			Selesai  : ` + strWaktuSelesai + `<br>
		</p>
		<div class="line"></div>
		<h3>TOTAL: Rp ` + fmt.Sprintf("%d", p.TotalBiaya) + `</h3>
		<div class="line"></div>
		<p class="center">
			Status: ` + p.Status + `<br>
			<i>Terima kasih telah mempercayakan<br>cucian Anda kepada kami!</i>
		</p>
	</body>
	</html>`

	// Balikin response-nya sebagai HTML
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlStruk))
}

// Fungsi buat Lihat Pesanan berdasarkan Id
func GetPesananById(w http.ResponseWriter, r *http.Request) {
	idPesanan := r.PathValue("id")

	// Cari pesanan di database
	var p models.Pesanan
	query := "SELECT id_pesanan, nama_customer, layanan, total_biaya, status FROM pesanan WHERE id_pesanan = $1"
	err := config.DB.QueryRow(query, idPesanan).Scan(&p.IDPesanan, &p.NamaCustomer, &p.Layanan, &p.TotalBiaya, &p.Status)

	if err != nil {
		http.Error(w, "Pesanan tidak ditemukan", http.StatusNotFound)
		return
	}

	var daftarPesanan []models.Pesanan
	daftarPesanan = append(daftarPesanan, p)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(daftarPesanan)
}
