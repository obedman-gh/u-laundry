package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"ulaundry/internal/config"
	"ulaundry/internal/migrate"
	"ulaundry/internal/platform"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (result error) {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	timeout := flags.Duration("timeout", 2*time.Minute, "batas durasi seluruh operasi (contoh 2m)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 1 || (flags.Arg(0) != "up" && flags.Arg(0) != "status") || *timeout <= 0 {
		return errors.New("penggunaan: migrate [-timeout 2m] up|status; rollback: lihat docs/database.md")
	}
	settings, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, *timeout)
	defer cancel()
	db, err := platform.OpenDatabase(ctx, settings)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	if flags.Arg(0) == "up" {
		if err := migrate.Up(ctx, db); err != nil {
			return err
		}
	}
	states, err := migrate.Status(ctx, db)
	if err != nil {
		return err
	}
	for _, state := range states {
		status := "pending"
		if state.AppliedAt != nil {
			status = "applied " + state.AppliedAt.UTC().Format(time.RFC3339)
		}
		fmt.Printf("%06d\t%s\t%s\n", state.Version, status, state.Name)
	}
	return nil
}
