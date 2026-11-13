package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/report"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func read(path string, out any) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil {
		return e
	}
	if len(b) > 1<<20 {
		return fmt.Errorf("report input exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(out); e != nil {
		return e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return fmt.Errorf("unexpected trailing report data")
	}
	return nil
}
func run() error {
	assessment := flag.String("assessment", ".local/assessment.json", "Controller assessment snapshot")
	diagnostic := flag.String("diagnostic", "", "optional persisted diagnostic report")
	output := flag.String("output", ".local/report.html", "private read-only HTML report")
	flag.Parse()
	var a monitor.Assessment
	if e := read(*assessment, &a); e != nil {
		return e
	}
	var d *diagnostics.Report
	if *diagnostic != "" {
		d = &diagnostics.Report{}
		if e := read(*diagnostic, d); e != nil {
			return e
		}
	}
	b, e := report.Render(a, d)
	if e != nil {
		return e
	}
	if e = durable.AtomicWrite(*output, b); e != nil {
		return e
	}
	fmt.Printf("Wrote read-only report to %s\n", *output)
	return nil
}
