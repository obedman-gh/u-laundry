package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const TrackingLifetime = 90 * 24 * time.Hour

// TrackingHash accepts only canonical 256-bit capability tokens. Entropy must
// come from crypto/rand at issuance; a parser cannot prove randomness.
func TrackingHash(token string) ([32]byte, bool) {
	if len(token) != 43 {
		return [32]byte{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(token)), true
}

// TrackingCode is a display reference, never a lookup key or internal order ID.
func TrackingCode(hash [32]byte) string { return hex.EncodeToString(hash[:6]) }

func TrackingMessage(status OrderStatus) string {
	switch status {
	case Received:
		return "Pesanan telah diterima."
	case Processing:
		return "Pesanan sedang diproses."
	case Ready:
		return "Pesanan siap diambil. Silakan hubungi layanan untuk pengambilan."
	case Collected:
		return "Pesanan telah diambil. Terima kasih."
	case Cancelled:
		return "Pesanan dibatalkan. Silakan hubungi layanan."
	default:
		return "Silakan hubungi layanan."
	}
}
