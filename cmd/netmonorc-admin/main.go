// netmonorc-admin performs local, one-time identity bootstrap; no HTTP bypass.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	database := flag.String("database-file", "", "private PostgreSQL DSN file")
	username := flag.String("username", "admin", "initial administrator username")
	stdin := flag.Bool("password-stdin", false, "read one password line from stdin instead of hidden terminal prompt")
	flag.Parse()
	if !auth.ValidUsername(*username) {
		return fmt.Errorf("invalid username")
	}
	dsn, err := os.ReadFile(*database)
	if err != nil {
		return fmt.Errorf("database file unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := storage.Open(ctx, strings.TrimSpace(string(dsn)))
	if err != nil {
		return err
	}
	defer store.Close()
	initialized, err := store.HasUsers(ctx)
	if err != nil {
		return err
	}
	if initialized {
		fmt.Println("Already initialized; credentials unchanged.")
		return nil
	}
	var password []byte
	if *stdin {
		password, err = io.ReadAll(io.LimitReader(os.Stdin, 259))
		if err != nil || len(password) > 258 {
			return fmt.Errorf("invalid password input")
		}
		password = []byte(strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r"))
		if strings.ContainsAny(string(password), "\r\n") {
			return fmt.Errorf("one password line required")
		}
	} else {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("terminal required; use password-stdin for protected input")
		}
		fmt.Fprint(os.Stderr, "Initial administrator password: ")
		password, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("password input unavailable")
		}
	}
	hash, err := auth.HashPassword(string(password))
	for i := range password {
		password[i] = 0
	}
	if err != nil {
		return err
	}
	created, err := store.BootstrapUser(ctx, *username, hash)
	if err != nil {
		return err
	}
	if created {
		fmt.Println("Initial administrator created.")
	} else {
		fmt.Println("Already initialized; credentials unchanged.")
	}
	return nil
}
