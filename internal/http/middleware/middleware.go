package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Middleware struct {
	jwtSecret     string
	allowedOrigin string
}

func New(jwtSecret, allowedOrigin string) *Middleware {
	if allowedOrigin == "" {
		allowedOrigin = "*" // Legacy default; production policy is tightened separately.
	}
	return &Middleware{jwtSecret: jwtSecret, allowedOrigin: allowedOrigin}
}

func (m *Middleware) BukaPintuCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", m.allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight request dari browser
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// POIN 7: Verifikasi Algoritma JWT
func (m *Middleware) ValidasiJWT(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Akses ditolak: Kasir belum login!", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		secret := m.jwtSecret
		if secret == "" {
			http.Error(w, "Error Server: JWT Secret belum dikonfigurasi", http.StatusInternalServerError)
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// VERIFIKASI KEAMANAN: Tolak token kalau algoritmanya bukan HMAC (Mencegah Bypass)
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("metode enkripsi tidak valid: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Akses ditolak: Token tidak valid/expired!", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}
