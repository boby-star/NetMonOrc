package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/boby-star/NetMonOrc/internal/apiclient"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
)

func diagnosticCommand(args []string) error {
	if len(args) < 2 || (args[1] != "run" && args[1] != "cancel" && args[1] != "export") {
		return fmt.Errorf("diagnostics run|cancel|export [flags] MONITOR|SESSION")
	}
	f := flag.NewFlagSet("diagnostics "+args[1], flag.ContinueOnError)
	output := f.String("output", "", "new private JSON export file (export only)")
	timeout := f.Duration("timeout", 2*time.Minute, "export deadline (1s..5m)")
	contextFile := f.String("context-file", "", "private PAT context")
	profile := f.String("profile", "", "explicit DiagnosticProfile name")
	requestID := f.String("request-id", "", "stable idempotency key for run; preserve on retry")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return fmt.Errorf("one Monitor or session ID required; flags precede the positional argument")
	}
	if args[1] == "run" && (*profile == "" || *requestID == "") {
		return fmt.Errorf("run requires profile and stable request-id")
	}
	if args[1] == "export" {
		if *output == "" || *profile != "" || *requestID != "" || *timeout < time.Second || *timeout > 5*time.Minute {
			return fmt.Errorf("export requires output and timeout 1s..5m, without profile/request-id")
		}
		if _, err := os.Lstat(*output); err == nil {
			return fmt.Errorf("export output already exists")
		} else if !os.IsNotExist(err) {
			return err
		}
	} else if *output != "" {
		return fmt.Errorf("output applies only to export")
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
	if args[1] == "export" {
		ctx, stop := context.WithTimeout(context.Background(), *timeout)
		defer stop()
		data, err := client.ExportDiagnostic(ctx, f.Arg(0))
		if err != nil {
			return err
		}
		if err = writeDiagnosticExport(*output, data); err != nil {
			return err
		}
		var summary apiclient.DiagnosticExport
		if err = json.Unmarshal(data, &summary); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Diagnostic evidence exported: %s (complete=%t, missing=%d)\n", *output, summary.Complete, len(summary.MissingObservationIDs))
		return nil
	}
	route := "/api/v1/diagnostics/" + url.PathEscape(f.Arg(0)) + "/cancel"
	var body any
	if args[1] == "run" {
		route = "/api/v1/monitors/" + url.PathEscape(f.Arg(0)) + "/diagnostics"
		body = diagnosticview.Start{ProfileRef: *profile, Reason: "manual", RequestID: *requestID}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data, err := client.JSON(ctx, "POST", route, body)
	if err != nil {
		return err
	}
	var check diagnosticview.Session
	if err = json.Unmarshal(data, &check); err != nil {
		return fmt.Errorf("invalid diagnostic response")
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}

// Publish a complete private file without clobbering an existing path.
func writeDiagnosticExport(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".diagnostic-export-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Link(file.Name(), path)
}
