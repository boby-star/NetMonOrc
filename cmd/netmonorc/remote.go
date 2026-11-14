package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/boby-star/NetMonOrc/internal/apiclient"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"gopkg.in/yaml.v3"
)

func cliContextPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config directory unavailable; provide context-file")
	}
	return filepath.Join(root, "netmonorc", "context.json"), nil
}
func remoteCommand(args []string) error {
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	file := f.String("f", "", "YAML manifest(s) or JSON/YAML ConfigBatch; - reads stdin")
	contextFile := f.String("context-file", "", "private CLI context")
	versionsFile := f.String("expected-versions", "", "JSON kind/name -> resourceVersion/null preconditions file")
	requestID := f.String("request-id", "", "stable idempotency key; preserve it and the batch on retry")
	wait := f.Bool("wait", false, "wait until required Agents acknowledge the operation")
	timeout := f.Duration("timeout", time.Minute, "overall deadline (1s..30m)")
	limit := f.Int("limit", 50, "read page size (1..200)")
	cursor := f.String("cursor", "", "opaque read page cursor")
	state := f.String("state", "", "operations state filter")
	_ = f.Bool("remote", false, "use server validation")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if *timeout < time.Second || *timeout > 30*time.Minute || *limit < 1 || *limit > 200 {
		return fmt.Errorf("invalid timeout or page limit")
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
	ctx, stop := context.WithTimeout(context.Background(), *timeout)
	defer stop()
	if args[0] == "get" {
		if f.NArg() < 1 || f.NArg() > 2 {
			return fmt.Errorf("get requires Monitor|DiagnosticProfile|operations|agents [name/id]; flags precede positional arguments")
		}
		var route string
		switch f.Arg(0) {
		case "Monitor":
			route = "/api/v1/resources/Monitor"
		case "DiagnosticProfile":
			route = "/api/v1/resources/DiagnosticProfile"
		case "operations":
			route = "/api/v1/operations"
		case "agents":
			route = "/api/v1/agents"
		default:
			return fmt.Errorf("this CLI supports remote get Monitor|DiagnosticProfile|operations|agents")
		}
		if f.NArg() == 2 {
			route += "/" + url.PathEscape(f.Arg(1))
		} else {
			params := url.Values{"limit": {fmt.Sprint(*limit)}}
			if *cursor != "" {
				params.Set("cursor", *cursor)
			}
			if *state != "" {
				params.Set("state", *state)
			}
			route += "?" + params.Encode()
		}
		data, err := client.JSON(ctx, "GET", route, nil)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, string(data))
		return err
	}
	if f.NArg() != 0 || *file == "" {
		return fmt.Errorf("%s requires -f path (or - for stdin)", args[0])
	}
	data, err := readCLIInput(*file)
	if err != nil {
		return err
	}
	batch, err := parseBatch(data)
	if err != nil {
		return err
	}
	if *versionsFile != "" {
		raw, err := readCLIInput(*versionsFile)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &batch.ExpectedVersions); err != nil {
			return fmt.Errorf("invalid expected-versions JSON map")
		}
	}
	if *requestID != "" {
		if batch.RequestID != "" && batch.RequestID != *requestID {
			return fmt.Errorf("request-id differs from the input batch")
		}
		batch.RequestID = *requestID
	}
	if args[0] == "apply" && batch.RequestID == "" {
		return fmt.Errorf("apply requires a stable requestId in ConfigBatch or --request-id; retain the identical batch for recovery")
	}
	route := "/api/v1/config/" + args[0]
	response, err := client.JSON(ctx, "POST", route, batch)
	if err != nil {
		return err
	}
	if args[0] == "validate" {
		var validation resources.Validation
		if json.Unmarshal(response, &validation) != nil {
			return fmt.Errorf("invalid validation response")
		}
		if _, err = fmt.Fprintln(os.Stdout, string(response)); err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("configuration validation failed")
		}
		return nil
	}
	if args[0] != "apply" || !*wait {
		_, err = fmt.Fprintln(os.Stdout, string(response))
		return err
	}
	var op resources.Operation
	if err = json.Unmarshal(response, &op); err != nil || op.ID == "" {
		return fmt.Errorf("invalid apply operation response")
	}
	fmt.Fprintf(os.Stderr, "Accepted operation %s; waiting for Agent acknowledgements.\n", op.ID)
	return waitOperation(ctx, client, op)
}
func readCLIInput(path string) ([]byte, error) {
	var reader io.Reader = os.Stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("input file unavailable")
		}
		defer file.Close()
		reader = file
	}
	b, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, fmt.Errorf("input unavailable or exceeds 1 MiB")
	}
	return b, nil
}

// parseBatch converts syntax only; domain validation remains on Controller.
func parseBatch(data []byte) (resources.Batch, error) {
	d := yaml.NewDecoder(bytes.NewReader(data))
	batch := resources.Batch{}
	for {
		var document any
		err := d.Decode(&document)
		if err == io.EOF {
			break
		}
		if err != nil {
			return batch, fmt.Errorf("invalid YAML/JSON input")
		}
		encoded, err := json.Marshal(document)
		if err != nil {
			return batch, fmt.Errorf("JSON-compatible YAML required")
		}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(encoded, &envelope) != nil {
			return batch, fmt.Errorf("resource or ConfigBatch object required")
		}
		if _, isBatch := envelope["manifests"]; isBatch {
			if len(batch.Manifests) > 0 {
				return batch, fmt.Errorf("ConfigBatch cannot be mixed with manifest documents")
			}
			decoder := json.NewDecoder(bytes.NewReader(encoded))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&batch); err != nil {
				return batch, fmt.Errorf("invalid ConfigBatch")
			}
			if d.Decode(new(any)) != io.EOF {
				return batch, fmt.Errorf("one ConfigBatch document required")
			}
			break
		}
		manifest, err := resources.Decode(encoded)
		if err != nil {
			return batch, err
		}
		batch.Manifests = append(batch.Manifests, manifest)
		if len(batch.Manifests) > 20 {
			return batch, fmt.Errorf("batch exceeds 20 manifests")
		}
	}
	if len(batch.Manifests) < 1 || len(batch.Manifests) > 20 {
		return batch, fmt.Errorf("expected 1..20 manifests")
	}
	return batch, nil
}
func waitOperation(ctx context.Context, client *apiclient.Client, op resources.Operation) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		switch op.State {
		case "completed":
			return json.NewEncoder(os.Stdout).Encode(op)
		case "failed":
			if err := json.NewEncoder(os.Stdout).Encode(op); err != nil {
				return err
			}
			return fmt.Errorf("operation %s failed", op.ID)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation %s remains pending; inspect it with get operations", op.ID)
		case <-tick.C:
		}
		data, err := client.JSON(ctx, "GET", "/api/v1/operations/"+url.PathEscape(op.ID), nil)
		if err != nil {
			var apiErr *apiclient.APIError
			if errors.As(err, &apiErr) && apiErr.Status != 429 && apiErr.Status != 503 {
				return err
			}
			continue
		}
		var next resources.Operation
		if err = json.Unmarshal(data, &next); err != nil || next.ID != op.ID {
			return fmt.Errorf("invalid operation status response")
		}
		op = next
	}
}
