package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"os/signal"
	"time"
	"ulaundry/internal/config"
	"ulaundry/internal/domain"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
	"ulaundry/internal/repository/postgres"
	"ulaundry/internal/service"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type bootstrapFunc func(context.Context, service.StaffInput) error

func run(ctx context.Context, args []string, in io.Reader, out, diagnostics io.Writer, bootstrap bootstrapFunc) error {
	if len(args) == 0 || args[0] != "bootstrap-owner" {
		return errors.New("penggunaan: admin bootstrap-owner -username USER -name NAMA [-password-stdin]")
	}
	flags := flag.NewFlagSet("bootstrap-owner", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	username := flags.String("username", "", "username owner pertama")
	name := flags.String("name", "", "nama owner pertama")
	piped := flags.Bool("password-stdin", false, "baca satu password dari pipe privat; jangan taruh password di argumen/history")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *username == "" || *name == "" {
		return errors.New("username dan name wajib diisi; password tidak diterima sebagai argumen")
	}
	var password []byte
	if *piped {
		if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			return errors.New("password-stdin memerlukan pipe privat, bukan terminal yang menampilkan input")
		}
		raw, err := io.ReadAll(io.LimitReader(in, 75))
		if err != nil {
			return errors.New("input password tidak dapat dibaca")
		}
		if len(raw) > 74 {
			return errors.New("password terlalu panjang")
		}
		password = bytes.TrimSuffix(bytes.TrimSuffix(raw, []byte("\n")), []byte("\r"))
	} else {
		f, ok := in.(*os.File)
		if !ok || !term.IsTerminal(int(f.Fd())) {
			return errors.New("terminal interaktif diperlukan; untuk otomasi gunakan -password-stdin dengan pipe privat")
		}
		fmt.Fprint(diagnostics, "Kata sandi owner baru (input tersembunyi): ")
		first, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(diagnostics)
		if err != nil {
			return errors.New("password tidak dapat dibaca")
		}
		fmt.Fprint(diagnostics, "Ulangi kata sandi: ")
		second, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(diagnostics)
		if err != nil {
			return errors.New("konfirmasi tidak dapat dibaca")
		}
		equal := subtle.ConstantTimeCompare(first, second) == 1
		clear(second)
		if !equal {
			clear(first)
			return errors.New("konfirmasi password tidak cocok")
		}
		password = first
	}
	defer clear(password)
	if err := domain.ValidateNewPassword(string(password)); err != nil {
		return err
	}
	if bootstrap == nil {
		bootstrap = bootstrapOwner
	}
	if err := bootstrap(ctx, service.StaffInput{Username: *username, Name: *name, Password: string(password)}); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "Owner pertama berhasil dibuat. Silakan masuk melalui halaman login.")
	return err
}
func bootstrapOwner(ctx context.Context, in service.StaffInput) (result error) {
	settings, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	db, err := platform.OpenDatabase(ctx, settings)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	states, err := migrate.Status(ctx, db)
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.AppliedAt == nil {
			return errors.New("migration belum lengkap; jalankan cmd/migrate up lebih dahulu")
		}
	}
	rt := service.Runtime{Clock: platform.SystemClock{}, IDs: platform.RandomIDs{}}
	_, err = service.NewStaffManagement(postgres.New(db), rt, platform.Passwords{}).Bootstrap(ctx, in)
	return err
}
