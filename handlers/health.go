package handlers

import (
	"net/http"
	"ulaundry/config"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	// Cek koneksi DB juga, kalau putus statusnya 500
	err := config.DB.Ping()
	if err != nil {
		http.Error(w, `{"status":"error", "database":"disconnected"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok", "version":"1.0.0", "database":"connected"}`))
}
