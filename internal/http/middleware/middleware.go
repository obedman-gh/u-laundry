package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Middleware struct {
	jwtSecret      string
	trustedOrigins map[string]bool
}

func New(jwtSecret string, trustedOrigins []string) *Middleware {
	origins := make(map[string]bool, len(trustedOrigins))
	for _, origin := range trustedOrigins {
		if origin != "" && origin != "*" {
			origins[origin] = true
		}
	}
	return &Middleware{jwtSecret: jwtSecret, trustedOrigins: origins}
}

func (m *Middleware) BukaPintuCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !m.trustedOrigins[origin] {
				http.Error(w, "Origin tidak diizinkan", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			method := r.Header.Get("Access-Control-Request-Method")
			if origin == "" || (method != "GET" && method != "POST" && method != "PUT") {
				http.Error(w, "Preflight tidak diizinkan", http.StatusForbidden)
				return
			}
			for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				name := strings.ToLower(strings.TrimSpace(header))
				if name != "" && name != "content-type" && name != "authorization" {
					http.Error(w, "Header preflight tidak diizinkan", http.StatusForbidden)
					return
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.WriteHeader(http.StatusNoContent)
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
