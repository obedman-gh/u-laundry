package domain

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func text(field, value string, max int, required, multiline bool) error {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > max || (required && value == "") {
		return invalid(field, "Teks wajib sesuai batas panjang dan tanpa spasi tepi.")
	}
	for _, r := range value {
		if multiline && (r == '\n' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return invalid(field, "Teks mengandung karakter yang tidak diizinkan.")
		}
	}
	return nil
}

func cleanNotes(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
}

func identifier(field, value string) error {
	if len(value) == 0 || len(value) > 100 {
		return invalid(field, "Identitas wajib diisi, maksimal 100 karakter.")
	}
	for _, r := range value {
		if !asciiAlphaNumeric(r) && r != '-' && r != '_' && r != '.' {
			return invalid(field, "Format identitas tidak valid.")
		}
	}
	return nil
}

func asciiAlphaNumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func timestamps(createdAt, updatedAt time.Time) error {
	if createdAt.IsZero() || updatedAt.IsZero() || updatedAt.Before(createdAt) {
		return invalid("timestamps", "Waktu wajib diisi dan pembaruan tidak boleh mendahului pembuatan.")
	}
	return nil
}

// NormalizeContact accepts a missing contact for walk-in customers. A supplied
// number is canonicalized; it does not prove ownership or WhatsApp availability.
func NormalizeContact(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var b strings.Builder
	for _, r := range raw {
		if r == ' ' || r == '-' || r == '(' || r == ')' {
			continue
		}
		if (r < '0' || r > '9') && r != '+' {
			return "", invalid("contact", "Nomor kontak tidak valid.")
		}
		b.WriteRune(r)
	}
	number := b.String()
	switch {
	case strings.HasPrefix(number, "0"):
		number = "+62" + number[1:]
	case strings.HasPrefix(number, "62"):
		number = "+" + number
	}
	if len(number) < 9 || len(number) > 16 || number[0] != '+' || number[1] == '0' {
		return "", invalid("contact", "Gunakan nomor lokal 0 atau format internasional + dengan 8–15 digit.")
	}
	for _, r := range number[1:] {
		if r < '0' || r > '9' {
			return "", invalid("contact", "Nomor kontak tidak valid.")
		}
	}
	return number, nil
}
