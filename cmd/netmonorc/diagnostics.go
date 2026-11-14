package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/boby-star/NetMonOrc/internal/apiclient"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
)

func diagnosticCommand(args []string) error {
	if len(args) < 2 || (args[1] != "run" && args[1] != "cancel") {
		return fmt.Errorf("diagnostics run|cancel [flags] MONITOR|SESSION")
	}
	f := flag.NewFlagSet("diagnostics "+args[1], flag.ContinueOnError)
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
