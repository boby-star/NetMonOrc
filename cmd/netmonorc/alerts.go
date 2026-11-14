package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/boby-star/NetMonOrc/internal/alertview"
	"github.com/boby-star/NetMonOrc/internal/apiclient"
)

func alertCommand(args []string) error {
	if len(args) < 2 || args[1] != "acknowledge" {
		return fmt.Errorf("alerts acknowledge [flags] ALERT_ID")
	}
	f := flag.NewFlagSet("alerts acknowledge", flag.ContinueOnError)
	contextFile := f.String("context-file", "", "private PAT context")
	noteFile := f.String("note-file", "", "optional UTF-8 note file; - reads stdin")
	expected := f.String("if-match", "", "optional opaque ETag; otherwise fetch current alert first")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return fmt.Errorf("one alert ID required; flags precede the positional argument")
	}
	note := ""
	if *noteFile != "" {
		input := io.Reader(os.Stdin)
		if *noteFile != "-" {
			file, err := os.Open(*noteFile)
			if err != nil {
				return fmt.Errorf("note file unavailable")
			}
			defer file.Close()
			input = file
		}
		data, err := io.ReadAll(io.LimitReader(input, 4097))
		if err != nil || len(data) > 4096 {
			return fmt.Errorf("invalid note or note exceeds 4096 UTF-8 bytes")
		}
		note = string(data)
	}
	path, err := cliContextPath(*contextFile)
	if err != nil {
		return err
	}
	saved, err := apiclient.Load(path)
	if err != nil {
		return err
	}
	client, err := saved.Client()
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	route := "/api/v1/alerts/" + url.PathEscape(f.Arg(0))
	tag := *expected
	if tag == "" {
		_, tag, err = client.VersionedJSON(ctx, "GET", route, nil, "")
		if err != nil {
			return err
		}
		if tag == "" {
			return fmt.Errorf("server did not return an alert version")
		}
	}
	data, _, err := client.VersionedJSON(ctx, "POST", route+"/acknowledge", alertview.Acknowledge{Note: note}, tag)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
