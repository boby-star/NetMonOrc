// Package coordinator connects the pure planner to durable admission and the
// existing Agent stream. This research profile admits one run per incident.
package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"github.com/boby-star/NetMonOrc/internal/transport"
)

type Coordinator struct {
	Plan       monitor.Plan
	Profile    diagnostics.Profile
	Server     *transport.Server
	Store      *storage.Store
	ReportPath string
	trigger    diagnostics.Trigger
	mode       string
	mu         sync.Mutex
	running    bool
	workers    sync.WaitGroup
	errors     chan error
}

func New(p monitor.Plan, profile diagnostics.Profile, s *transport.Server, store *storage.Store, path string) (*Coordinator, error) {
	if p.Diagnostics == nil || !p.Diagnostics.Enabled || p.Diagnostics.ProfileHash == "" || p.Diagnostics.ProfileRef != profile.Name {
		return nil, fmt.Errorf("resolved enabled diagnostics required")
	}
	if profile.Budget.MaxEstimatedBytes != 0 {
		return nil, fmt.Errorf("live diagnostic byte estimates are not measured; byte budget unsupported")
	}
	for _, a := range profile.Actions {
		if a.Check.Timeout > 15*time.Second {
			return nil, fmt.Errorf("research diagnostic timeout must be <=15s to fit a renewed 30s lease with startup/clock bounds")
		}
	}
	t, e := diagnostics.CompileTrigger(p.Diagnostics.When)
	if e != nil {
		return nil, e
	}
	mode := profile.Mode
	if p.Diagnostics.ComparisonStatic {
		mode = "static"
	}
	return &Coordinator{mode: mode, Plan: p, Profile: profile, Server: s, Store: store, ReportPath: path, trigger: t, errors: make(chan error, 1)}, nil
}
func (c *Coordinator) Tick(ctx context.Context, a monitor.Assessment, observations []monitor.Observation) error {
	select {
	case e := <-c.errors:
		return e
	default:
	}
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()
	if running {
		return nil
	}
	if a.State == "healthy" {
		return c.Store.CloseDiagnosticEpisode(ctx, a)
	}
	permitted := false
	for _, s := range c.Plan.Diagnostics.OnStates {
		if s == a.State {
			permitted = true
		}
	}
	if !permitted {
		return nil
	}
	yes, e := c.trigger(ctx, a)
	if e != nil || !yes {
		return nil
	}
	agents := c.Server.Executors(time.Now())
	// Wait for a usable catalog instead of spending the sole incident session
	// while no authenticated executor has reported capacity.
	if len(agents) == 0 {
		return nil
	}
	started := time.Now()
	dbctx, done := context.WithTimeout(ctx, 5*time.Second)
	id, e := c.Store.StartDiagnostic(dbctx, c.Plan, c.Profile)
	done()
	if e != nil {
		return e
	}
	if id == "" {
		return nil
	}
	initial := diagnostics.MatchBaseline(c.Plan, c.Profile, observations, agents, time.Now())
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		defer func() { c.mu.Lock(); c.running = false; c.mu.Unlock() }()
		runner := transport.DiagnosticRunner{Server: c.Server, RunID: id, Plan: c.Plan}
		engine := diagnostics.Engine{TaskOverhead: 20 * time.Second, Runner: runner, Eligible: func(action diagnostics.Action, agent diagnostics.Executor) bool {
			return agent.Supported["diagnostics:serial"] && (len(action.Check.Extract) == 0 || agent.Supported["extraction:regex"])
		}}
		result, e := engine.Run(ctx, c.Profile, c.mode, agents, &diagnostics.Ledger{StartedAt: started}, initial, a.State)
		if e != nil {
			result.Status = "inconclusive"
			result.Reason = "planner_error"
		}
		persist, finish := context.WithTimeout(context.Background(), 5*time.Second)
		defer finish()
		if err := c.Store.FinishDiagnostic(persist, id, result); err != nil {
			c.fail(err)
			return
		}
		if err := c.RestoreReport(persist); err != nil {
			c.fail(err)
		}
	}()
	return nil
}
func (c *Coordinator) fail(e error) {
	select {
	case c.errors <- e:
	default:
	}
}
func (c *Coordinator) Wait() { c.workers.Wait() }

func (c *Coordinator) RestoreReport(ctx context.Context) error {
	r, e := c.Store.LatestDiagnostic(ctx, c.Plan.Name)
	if e != nil {
		return e
	}
	if r == nil {
		return nil
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	return durable.AtomicWrite(c.ReportPath, b)
}
