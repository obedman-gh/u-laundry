package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateNewPassword is for account creation/reset, never a login restriction.
// Bcrypt's 72-byte limit is enforced before hashing, without silent truncation.
func ValidateNewPassword(value string) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 12 || len(value) > 72 || strings.TrimSpace(value) != value {
		return invalid("password", "Gunakan frasa sandi 12 karakter atau lebih, maksimal 72 byte, tanpa spasi tepi.")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return invalid("password", "Kata sandi mengandung karakter yang tidak diizinkan.")
		}
	}
	normalized := strings.ToLower(value)
	for _, weak := range []string{"123456789012", "password1234", "password12345", "password123!", "ulaundry12345", "qwerty123456", "administrator"} {
		if normalized == weak {
			return invalid("password", "Gunakan frasa sandi yang tidak umum.")
		}
	}
	if strings.Count(value, string([]rune(value)[0])) == utf8.RuneCountInString(value) {
		return invalid("password", "Gunakan frasa sandi yang tidak berulang.")
	}
	return nil
}
