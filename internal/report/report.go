// Package report renders a private, self-contained read-only research report.
// It presents server assessments and never reimplements assessment in a browser.
package report

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"time"

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

//go:embed report.html
var document string

type View struct {
	Assessment           monitor.Assessment
	Diagnostic           *diagnostics.Report
	GeneratedAt          time.Time
	HistoricalDiagnostic bool
	Alert                *alerting.State
	Delivery             *webhook.Summary
}

func Render(a monitor.Assessment, d *diagnostics.Report) ([]byte, error) {
	return RenderWithAlert(a, d, nil)
}

func RenderWithAlert(a monitor.Assessment, d *diagnostics.Report, alert *alerting.State) ([]byte, error) {
	return RenderWithDelivery(a, d, alert, nil)
}

func RenderWithDelivery(a monitor.Assessment, d *diagnostics.Report, alert *alerting.State, delivery *webhook.Summary) ([]byte, error) {
	if delivery != nil && delivery.Monitor != a.Monitor {
		return nil, fmt.Errorf("delivery summary belongs to another Monitor")
	}
	if a.Monitor == "" || a.At.IsZero() {
		return nil, fmt.Errorf("assessment name/time required")
	}
	if d != nil && d.Monitor != a.Monitor {
		return nil, fmt.Errorf("diagnostic report belongs to another Monitor")
	}
	funcs := template.FuncMap{
		"state": func(s string) string {
			if x, ok := map[string]string{"healthy": "Працює", "degraded": "Погіршення", "down": "Відмова", "unknown": "Недостатньо доказів", "completed": "Завершено", "inconclusive": "Без однозначного висновку", "running": "Виконується"}[s]; ok {
				return x
			}
			return s
		},
		"class": func(s string) string {
			switch s {
			case "healthy", "completed":
				return "good"
			case "down":
				return "bad"
			case "degraded":
				return "warn"
			}
			return "muted"
		},
		"percent": func(v float64) string { return fmt.Sprintf("%.0f%%", 100*v) },
		"score": func(v *float64) string {
			if v == nil {
				return "—"
			}
			return fmt.Sprintf("%.0f%%", 100*(*v))
		},
		"stamp": func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
		"coverage": func(s string) string {
			if s == "complete" {
				return "Повне"
			}
			return "Неповне"
		},
	}
	t, e := template.New("report").Funcs(funcs).Parse(document)
	if e != nil {
		return nil, e
	}
	var out bytes.Buffer
	historical := d != nil && d.Revision != a.Revision
	e = t.Execute(&out, View{Assessment: a, Diagnostic: d, GeneratedAt: time.Now().UTC(), HistoricalDiagnostic: historical, Alert: alert, Delivery: delivery})
	if e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}
