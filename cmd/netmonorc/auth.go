package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/apiclient"
	"golang.org/x/term"
)

func authCommand(args []string) error {
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	server := f.String("server", "", "Controller HTTPS origin (login)")
	caFile := f.String("ca-file", "", "additional trusted CA PEM (login)")
	contextFile := f.String("context-file", "", "private context file; default user config/netmonorc/context.json")
	stdin := f.Bool("token-stdin", false, "read personal token from protected stdin (login)")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	path := *contextFile
	if path == "" {
		root, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("user config directory unavailable; provide context-file")
		}
		path = filepath.Join(root, "netmonorc", "context.json")
	}
	if args[0] != "login" && (*server != "" || *caFile != "" || *stdin) {
		return fmt.Errorf("server, ca-file and token-stdin apply only to login")
	}
	if args[0] == "logout" {
		if err := apiclient.Logout(path); err != nil {
			return err
		}
		fmt.Println("Local CLI credentials removed. Remote token remains valid until expiry or revocation in the panel.")
		return nil
	}
	if args[0] == "whoami" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := apiclient.Load(path)
		if err != nil {
			return err
		}
		view, err := c.WhoAmI(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(view)
	}
	if *server == "" {
		return fmt.Errorf("login requires --server https://host:port")
	}
	var input []byte
	var err error
	if *stdin {
		input, err = io.ReadAll(io.LimitReader(os.Stdin, 130))
		if err != nil || len(input) > 129 {
			return fmt.Errorf("invalid token input")
		}
		input = []byte(strings.TrimSuffix(strings.TrimSuffix(string(input), "\n"), "\r"))
	} else {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("terminal required; use --token-stdin for protected input")
		}
		fmt.Fprint(os.Stderr, "Personal API token: ")
		input, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("token input unavailable")
		}
	}
	defer func() {
		for i := range input {
			input[i] = 0
		}
	}()
	c := apiclient.Context{Server: *server, Token: string(input)}
	if *caFile != "" {
		c.CAFile, err = filepath.Abs(*caFile)
		if err != nil {
			return fmt.Errorf("invalid CA file path")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	view, err := c.WhoAmI(ctx)
	if err != nil {
		return err
	}
	if err = apiclient.Save(path, c); err != nil {
		return err
	}
	fmt.Printf("Authenticated as %s (%s). Private CLI context saved.\n", view.Username, view.Role)
	return nil
}
