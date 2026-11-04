package handlers

import (
	"encoding/json"
	"net/http"
	"ulaundry/config"
)

func GetNeraca(w http.ResponseWriter, r *http.Request) {
	var totalDebit, totalKredit int

	// COALESCE buat mastiin kalau datanya kosong, dia ngereturn 0, bukan error/nil
	config.DB.QueryRow("SELECT COALESCE(SUM(nominal), 0) FROM keuangan WHERE tipe = 'DEBIT'").Scan(&totalDebit)
	config.DB.QueryRow("SELECT COALESCE(SUM(nominal), 0) FROM keuangan WHERE tipe = 'KREDIT'").Scan(&totalKredit)

	saldoAkhir := totalDebit - totalKredit

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_pemasukan":   totalDebit,
		"total_pengeluaran": totalKredit,
		"saldo_akhir":       saldoAkhir,
	})
}
