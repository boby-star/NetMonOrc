// Package dashboard defines a scoped read projection, not another assessment engine.
package dashboard

import (
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"time"
)

type Reads struct{ Resources, Agents, Alerts, Diagnostics bool }
type Source struct {
	Monitors    int
	Assessment  *monitor.Assessment
	Firing      int
	Diagnostics []diagnosticview.Session
}
type Summary struct {
	ObservedAt        time.Time                 `json:"observedAt"`
	MonitorStates     map[string]int            `json:"monitorStates,omitempty"`
	CoverageStates    map[string]int            `json:"coverageStates,omitempty"`
	AgentCounts       map[string]int            `json:"agentCounts,omitempty"`
	FiringAlerts      *int                      `json:"firingAlerts,omitempty"`
	RecentDiagnostics *[]diagnosticview.Session `json:"recentDiagnostics,omitempty"`
}
