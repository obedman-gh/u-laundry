package platform

import (
	"strings"
	"testing"
)

func TestNewPasswordPolicyAndExistingLogin(t *testing.T) {
	p := Passwords{}
	for _, value := range []string{"short", "123456789012", strings.Repeat("a", 12), strings.Repeat("é", 37), " long passphrase ", "long\x00passphrase"} {
		if _, err := p.Hash(value); err == nil {
			t.Fatal("weak/unsupported password accepted")
		}
	}
	hash, err := p.Hash("Frasa sandi sintetis 2026")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Compare(hash, "Frasa sandi sintetis 2026") || p.Compare(hash, "incorrect") || p.Compare("", "incorrect") {
		t.Fatal("password verification failed")
	}
}
