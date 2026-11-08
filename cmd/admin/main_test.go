package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"ulaundry/internal/service"
)

func TestBootstrapCommandPrivateInput(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		args       []string
		valid      bool
	}{{"valid", "Frasa sandi sintetis\n", []string{"bootstrap-owner", "-username", "owner.test", "-name", "Owner", "-password-stdin"}, true}, {"not-interactive", "Frasa sandi sintetis", []string{"bootstrap-owner", "-username", "owner.test", "-name", "Owner"}, false}, {"oversized", strings.Repeat("x", 75), []string{"bootstrap-owner", "-username", "owner.test", "-name", "Owner", "-password-stdin"}, false}, {"no-command", "", nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			calls := 0
			err := run(context.Background(), tc.args, strings.NewReader(tc.body), &out, &diagnostics, func(_ context.Context, in service.StaffInput) error {
				calls++
				if in.Password != "Frasa sandi sintetis" || in.Username != "owner.test" {
					t.Fatal("wrong input")
				}
				return nil
			})
			if (err == nil) != tc.valid || tc.valid && calls != 1 || !tc.valid && calls != 0 {
				t.Fatal("command contract", err)
			}
			if strings.Contains(out.String(), "Frasa") || strings.Contains(diagnostics.String(), "Frasa") {
				t.Fatal("password printed")
			}
		})
	}
}
