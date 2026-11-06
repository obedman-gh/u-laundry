package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"

	"ulaundry/models"
)

// Pindahin fungsi getSemuaPesanan dari main.go kemaren
func (h *Handler) GetSemuaPesanan(w http.ResponseWriter, r *http.Request) {
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
	rows, err := h.db.Query(query)

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
func (h *Handler) BikinPesananBaru(w http.ResponseWriter, r *http.Request) {
	var p models.Pesanan
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Format JSON salah", http.StatusBadRequest)
		return
	}

	// Cari bagian query INSERT di BikinPesananBaru, lalu ganti jadi:
	query := `INSERT INTO pesanan (id_pesanan, nama_customer, layanan, total_biaya, waktu_perkiraan_selesai) 
    	      VALUES ($1, $2, $3, $4, $5)`
	_, err := h.db.Exec(query, p.IDPesanan, p.NamaCustomer, p.Layanan, p.TotalBiaya, p.WaktuPerkiraanSelesai)
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
func (h *Handler) UpdateStatusPesanan(w http.ResponseWriter, r *http.Request) {
	idPesanan := r.PathValue("id")

	var input struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Format JSON salah", http.StatusBadRequest)
		return
	}

	// 1. MULAI TRANSAKSI DATABASE (Poin 4)
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, "Gagal memulai transaksi server", http.StatusInternalServerError)
		return
	}
	// Kalau nanti ada error di tengah jalan, otomatis di-Rollback (dibatalkan semua)
	defer tx.Rollback()

	// 2. CEK STATUS LAMA & TOTAL BIAYA (Buat Poin 5)
	var statusLama string
	var totalBiaya int
	err = tx.QueryRow("SELECT status, total_biaya FROM pesanan WHERE id_pesanan = $1", idPesanan).Scan(&statusLama, &totalBiaya)
	if err != nil {
		http.Error(w, "Pesanan tidak ditemukan", http.StatusNotFound)
		return
	}

	// 3. UPDATE STATUS PESANAN (Sekaligus nangkep waktu selesai kalau statusnya pas)
	queryUpdate := `UPDATE pesanan SET status = $1`
	if input.Status == "Selesai & Siap Diambil" {
		queryUpdate += `, waktu_selesai_real = CURRENT_TIMESTAMP`
	}
	queryUpdate += ` WHERE id_pesanan = $2`

	_, err = tx.Exec(queryUpdate, input.Status, idPesanan)
	if err != nil {
		http.Error(w, "Gagal update database pesanan", http.StatusInternalServerError)
		return // Terjadi error, defer tx.Rollback() akan otomatis jalan
	}

	// 4. LOGIKA KEUANGAN AMAN (Mencegah Double Debit)
	// Hanya catat ke buku kas KALAU status barunya "Sudah Lunas" DAN status lamanya BUKAN "Sudah Lunas"
	if input.Status == "Sudah Lunas" && statusLama != "Sudah Lunas" {
		_, err = tx.Exec("INSERT INTO keuangan (tipe, keterangan, nominal) VALUES ('DEBIT', $1, $2)", "Pembayaran "+idPesanan, totalBiaya)
		if err != nil {
			http.Error(w, "Gagal mencatat buku kas keuangan", http.StatusInternalServerError)
			return // Terjadi error, status lunas di tabel pesanan juga ikut DIBATALKAN karena satu transaksi
		}
	}

	// 5. COMMIT TRANSAKSI (Simpan permanen ke database)
	if err = tx.Commit(); err != nil {
		http.Error(w, "Gagal menyimpan transaksi", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"pesan": "Status pesanan " + idPesanan + " berhasil diubah jadi " + input.Status,
	})
}

// Fungsi buat Filter pesanan berdasarkan status (GET Request)
func (h *Handler) GetPesananByStatus(w http.ResponseWriter, r *http.Request) {
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
	rows, err := h.db.Query(query)

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
func (h *Handler) CetakStruk(w http.ResponseWriter, r *http.Request) {
	idPesanan := r.PathValue("id")

	// Cari pesanan di database
	var p models.Pesanan
	query := "SELECT id_pesanan, nama_customer, layanan, total_biaya, status, TO_CHAR(waktu_pesan, 'YYYY-MM-DD HH24:MI:SS'), TO_CHAR(waktu_perkiraan_selesai, 'YYYY-MM-DD HH24:MI:SS'), TO_CHAR(waktu_selesai_real, 'YYYY-MM-DD HH24:MI:SS') FROM pesanan WHERE id_pesanan = $1"

	err := h.db.QueryRow(query, idPesanan).Scan(&p.IDPesanan, &p.NamaCustomer, &p.Layanan, &p.TotalBiaya, &p.Status, &p.WaktuPesan, &p.WaktuPerkiraanSelesai, &p.WaktuSelesaiReal)

	if err != nil {
		http.Error(w, "Pesanan tidak ditemukan", http.StatusNotFound)
		return
	}

	// --- CUKUP UBAH BAGIAN TENGAH FUNGSI CetakStruk ---
	// Poin 10: HTML Escaping (Membersihkan inputan jahat dari hacker)
	safeID := html.EscapeString(p.IDPesanan)
	safeNama := html.EscapeString(p.NamaCustomer)
	safeLayanan := html.EscapeString(p.Layanan)
	safeStatus := html.EscapeString(p.Status)

	strWaktuSelesai := "-"
	if p.WaktuSelesaiReal != nil {
		strWaktuSelesai = html.EscapeString(*p.WaktuSelesaiReal)
	}
	safeWaktuPesan := html.EscapeString(p.WaktuPesan)

	// Bikin desain struk HTML
	htmlStruk := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Struk ` + safeID + `</title>
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
			ID Order : ` + safeID + `<br>
			Nama     : ` + safeNama + `<br>
			Layanan  : ` + safeLayanan + `<br>
			Mulai    : ` + safeWaktuPesan + `<br>
			Selesai  : ` + strWaktuSelesai + `<br>
		</p>
		<div class="line"></div>
		<h3>TOTAL: Rp ` + fmt.Sprintf("%d", p.TotalBiaya) + `</h3>
		<div class="line"></div>
		<p class="center">
			Status: ` + safeStatus + `<br>
			<i>Terima kasih telah mempercayakan<br>cucian Anda kepada kami!</i>
		</p>
	</body>
	</html>`
	// ----------------------------------------------------

	// Balikin response-nya sebagai HTML
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlStruk))
}

// Fungsi buat Lihat Pesanan berdasarkan Id
func (h *Handler) GetPesananById(w http.ResponseWriter, r *http.Request) {
	idPesanan := r.PathValue("id")

	// Cari pesanan di database
	var p models.Pesanan
	query := "SELECT id_pesanan, nama_customer, layanan, total_biaya, status FROM pesanan WHERE id_pesanan = $1"
	err := h.db.QueryRow(query, idPesanan).Scan(&p.IDPesanan, &p.NamaCustomer, &p.Layanan, &p.TotalBiaya, &p.Status)

	if err != nil {
		http.Error(w, "Pesanan tidak ditemukan", http.StatusNotFound)
		return
	}

	var daftarPesanan []models.Pesanan
	daftarPesanan = append(daftarPesanan, p)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(daftarPesanan)
}
