// netmonorc-experiment replays an explicit synthetic outcome matrix. It does
// not run probes, trigger operational diagnostics or bypass Monitor.enabled.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/durable"
)

type scenario struct {
	Name     string                         `json:"name"`
	Truth    string                         `json:"truth"`
	Outcomes map[string]diagnostics.Outcome `json:"outcomes"`
}
type dataset struct {
	Executors []diagnostics.Executor `json:"executors"`
	Scenarios []scenario             `json:"scenarios"`
}
type replay struct {
	outcomes map[string]diagnostics.Outcome
}

func (r replay) Execute(_ context.Context, a diagnostics.Action, e diagnostics.Executor) (diagnostics.Outcome, error) {
	o, ok := r.outcomes[a.ID+"@"+e.ID]
	if !ok {
		return o, fmt.Errorf("no recorded outcome for selected pair")
	}
	return o, nil
}

type record struct {
	Scenario string             `json:"scenario"`
	Truth    string             `json:"truth"`
	Result   diagnostics.Result `json:"result"`
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func read(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 1<<20 {
		return nil, fmt.Errorf("input exceeds 1 MiB")
	}
	return b, nil
}
func run() error {
	adaptivePath := flag.String("adaptive", "examples/diagnostics/adaptive.yaml", "adaptive profile")
	fixedPath := flag.String("fixed", "examples/diagnostics/fixed.yaml", "fixed profile")
	casesPath := flag.String("scenarios", "examples/diagnostics/scenarios.json", "explicit replay matrix, not live observations")
	output := flag.String("output", ".local/diagnostics-replay.json", "JSON report; sibling CSV is also written")
	flag.Parse()
	b, e := read(*adaptivePath)
	if e != nil {
		return e
	}
	adaptive, e := config.CompileDiagnostics(b)
	if e != nil {
		return e
	}
	b, e = read(*fixedPath)
	if e != nil {
		return e
	}
	fixed, e := config.CompileDiagnostics(b)
	if e != nil {
		return e
	}
	if adaptive.Mode != "adaptive" || fixed.Mode != "fixed" {
		return fmt.Errorf("profile modes do not match requested baselines")
	}
	catalog := func(p diagnostics.Profile) []diagnostics.Action {
		a := append([]diagnostics.Action(nil), p.Actions...)
		sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
		return a
	}
	hypotheses := func(p diagnostics.Profile) []string {
		h := append([]string(nil), p.Hypotheses...)
		sort.Strings(h)
		return h
	}
	if !reflect.DeepEqual(catalog(adaptive), catalog(fixed)) || adaptive.Budget != fixed.Budget || !reflect.DeepEqual(hypotheses(adaptive), hypotheses(fixed)) {
		return fmt.Errorf("comparison requires identical catalogs, hypotheses and budgets")
	}
	b, e = read(*casesPath)
	if e != nil {
		return e
	}
	var data dataset
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&data); e != nil {
		return e
	}
	if len(data.Scenarios) == 0 || len(data.Scenarios) > 100 || len(data.Executors) == 0 || len(data.Executors) > 100 {
		return fmt.Errorf("unsupported scenario/executor count")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var rows []record
	var csvBuffer bytes.Buffer
	writer := csv.NewWriter(&csvBuffer)
	if e = writer.Write([]string{"scenario", "truth", "mode", "status", "reason", "predicted_class", "attempts", "cost_units"}); e != nil {
		return e
	}
	for _, s := range data.Scenarios {
		if s.Name == "" || s.Truth == "" {
			return fmt.Errorf("scenario name/truth required")
		}
		for _, mode := range []string{"static", "fixed", "adaptive"} {
			p := adaptive
			if mode == "fixed" {
				p = fixed
			}
			// Eligibility is declared by this synthetic experiment only. This
			// gate must never be reused as a production network authorization.
			engine := diagnostics.Engine{Runner: replay{s.Outcomes}, Eligible: func(diagnostics.Action, diagnostics.Executor) bool { return true }}
			result, e := engine.Run(ctx, p, mode, data.Executors, &diagnostics.Ledger{}, nil, "down")
			if e != nil {
				return e
			}
			rows = append(rows, record{s.Name, s.Truth, result})
			prediction := ""
			if result.Status == "completed" && len(result.Hypotheses) == 1 {
				prediction = result.Hypotheses[0]
			}
			if e = writer.Write([]string{s.Name, s.Truth, mode, result.Status, result.Reason, prediction, strconv.Itoa(result.Attempts), strconv.FormatFloat(result.Cost, 'g', -1, 64)}); e != nil {
				return e
			}
		}
	}
	writer.Flush()
	if e = writer.Error(); e != nil {
		return e
	}
	report := struct {
		Source string   `json:"source"`
		Rows   []record `json:"rows"`
	}{"synthetic-replay-not-network-performance", rows}
	b, e = json.MarshalIndent(report, "", "  ")
	if e != nil {
		return e
	}
	if e = durable.AtomicWrite(*output, b); e != nil {
		return e
	}
	csvPath := strings.TrimSuffix(*output, filepath.Ext(*output)) + ".csv"
	if e = durable.AtomicWrite(csvPath, csvBuffer.Bytes()); e != nil {
		return e
	}
	fmt.Printf("Wrote %d replay results to %s and %s; no probes were executed\n", len(rows), *output, csvPath)
	return nil
}
