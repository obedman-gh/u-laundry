package config

import (
	"testing"
	"time"
)

func TestAuthenticationConfiguration(t *testing.T) {
	c, err := parseMap(developmentEnv())
	if err != nil || c.JWTIssuer != c.PublicBaseURL || c.JWTAudience != "ulaundry-staff" || c.SessionLifetime != 8*time.Hour {
		t.Fatal("unsafe defaults")
	}
	for _, change := range [][2]string{{"JWT_SECRET", ""}, {"COOKIE_SECRET", ""}, {"SESSION_TTL", "14m"}, {"SESSION_TTL", "13h"}, {"SESSION_TTL", "15m1ms"}, {"JWT_ISSUER", " "}, {"JWT_AUDIENCE", " "}, {"JWT_ISSUER", "invalid\x00issuer"}, {"JWT_ISSUER", "invalid issuer"}, {"JWT_AUDIENCE", "invalid audience"}, {"LOGIN_RATE_LIMIT", "0"}, {"LOGIN_RATE_LIMIT", "101"}, {"LOGIN_RATE_WINDOW", "0s"}, {"LOGIN_RATE_WINDOW", "2h"}} {
		env := developmentEnv()
		env[change[0]] = change[1]
		if _, err := parseMap(env); err == nil {
			t.Fatalf("unsafe %s accepted", change[0])
		}
	}
}
