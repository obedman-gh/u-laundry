package platform

import (
	"encoding/base64"
	"testing"
	"ulaundry/internal/domain"
)

func TestTrackingRandomTokens256BitAndUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 4096; i++ {
		token, err := (RandomTokens{}).New()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || len(raw)*8 != 256 || seen[token] {
			t.Fatal("wrong size or repeated random capability")
		}
		if _, ok := domain.TrackingHash(token); !ok {
			t.Fatal("noncanonical generator output")
		}
		seen[token] = true
	}
	// Sampling detects regressions; security comes from crypto/rand, not this count.
}
