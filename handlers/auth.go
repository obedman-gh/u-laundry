package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"ulaundry/config"
	"ulaundry/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	// godotenv dihapus dari sini karena udah diload di config/database.go
)

func Login(w http.ResponseWriter, r *http.Request) {
	var input models.Kasir
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Format input salah", http.StatusBadRequest)
		return
	}

	// 1. Cari Kasir di Database
	var dbPassword, namaLengkap string
	err := config.DB.QueryRow("SELECT password, nama_lengkap FROM kasir WHERE username = $1", input.Username).Scan(&dbPassword, &namaLengkap)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Username tidak ditemukan", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Error server", http.StatusInternalServerError)
		return
	}

	// 2. Cocokkin Password Asli dengan yang ada di DB (Bcrypt)
	err = bcrypt.CompareHashAndPassword([]byte(dbPassword), []byte(input.Password))
	if err != nil {
		http.Error(w, "Password salah bro!", http.StatusUnauthorized)
		return
	}

	// 3. Kalo sukses, kita bikin Tiket JWT (Berlaku 24 jam)
	expirationTime := time.Now().Add(24 * time.Hour)

	// Data yang mau diselipin di tiket
	claims := &jwt.MapClaims{
		"username":     input.Username,
		"nama_lengkap": namaLengkap,
		"exp":          expirationTime.Unix(),
	}

	// 4. Ambil Kunci Rahasia dari .env pas mau tanda tangan tiket aja
	jwtKey := []byte(os.Getenv("JWT_SECRET"))

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		http.Error(w, "Gagal bikin token", http.StatusInternalServerError)
		return
	}

	// 5. Kirim tiketnya ke frontend
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"pesan": "Login Sukses!",
		"token": tokenString,
		"nama":  namaLengkap,
	})
}
