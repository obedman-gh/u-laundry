package handlers

import (
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Middleware CORS: Fungsi yang ngebungkus fungsi lain
func BukaPintuCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Kasih izin ke semua domain (pake tanda bintang *).
		// Nanti pas beneran online, bintangnya lu ganti jadi "https://ulaundry.obedman.my.id" biar lebih aman.
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// 2. Kasih tau method apa aja yang boleh dipake
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		// 3. Kasih tau header apa aja yang boleh dikirim (kayak Content-Type buat JSON)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 4. Kalo browser ngecek preflight (nanya dulu pake method OPTIONS sebelum ngirim data beneran),
		// langsung kita kasih OK tanpa nerusin ke fungsi utama.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 5. Kalau semua aman, silakan masuk ke fungsi utama (next)
		next.ServeHTTP(w, r)
	}
}

// ... (kode BukaPintuCORS yang kemaren biarin aja di atas) ...

// Middleware JWT: Satpam kedua buat ngecek tiket
func ValidasiJWT(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Cek apakah ada tiket (token) di header "Authorization"
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Akses ditolak: Kasir belum login!", http.StatusUnauthorized)
			return
		}

		// 2. Potong tulisan "Bearer " buat ngambil token aslinya
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// 3. Validasi keaslian token pake kunci rahasia kita
		jwtKey := []byte(os.Getenv("JWT_SECRET"))

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil // JWTKey ini sama dengan jwtKey file auth.go lu
		})

		// 4. Kalo token palsu atau udah kadaluarsa
		if err != nil || !token.Valid {
			http.Error(w, "Akses ditolak: Token tidak valid/expired!", http.StatusUnauthorized)
			return
		}

		// 5. Kalo token asli, silakan lewat ke fungsi utama
		next.ServeHTTP(w, r)
	}
}
