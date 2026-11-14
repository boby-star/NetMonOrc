package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/boby-star/NetMonOrc/internal/apiclient"
	"github.com/boby-star/NetMonOrc/internal/operationview"
)

func webhookCommand(args []string) error {
	if len(args) < 2 || args[1] != "test" {
		return fmt.Errorf("webhook test [flags]")
	}
	f := flag.NewFlagSet("webhook test", flag.ContinueOnError)
	contextFile := f.String("context-file", "", "private PAT context")
	wait := f.Bool("wait", false, "wait for durable delivery outcome")
	timeout := f.Duration("timeout", time.Minute, "overall timeout (1s..5m)")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 || *timeout < time.Second || *timeout > 5*time.Minute {
		return fmt.Errorf("invalid webhook test arguments")
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
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	data, err := client.JSON(ctx, "POST", "/api/v1/webhook/test", struct{}{})
	if err != nil {
		return err
	}
	var op operationview.Operation
	if err = json.Unmarshal(data, &op); err != nil || op.Kind != "webhook.test" {
		return fmt.Errorf("invalid webhook operation response")
	}
	for *wait && op.State != "completed" && op.State != "failed" {
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("delivery is not confirmed; inspect operation %s", op.ID)
		case <-timer.C:
		}
		data, err = client.JSON(ctx, "GET", "/api/v1/operations/"+op.ID, nil)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &op); err != nil {
			return fmt.Errorf("invalid operation status")
		}
	}
	if _, err = fmt.Fprintln(os.Stdout, string(data)); err != nil {
		return err
	}
	if op.State == "failed" {
		return fmt.Errorf("webhook test failed; inspect its operation")
	}
	return nil
}
