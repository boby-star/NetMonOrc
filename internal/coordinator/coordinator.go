// Package coordinator connects the pure planner to durable admission and the
// existing Agent stream. This research profile admits one run per incident.
package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"github.com/boby-star/NetMonOrc/internal/transport"
)

type Coordinator struct {
	Plan         monitor.Plan
	Profile      diagnostics.Profile
	Server       *transport.Server
	Store        *storage.Store
	ReportPath   string
	trigger      diagnostics.Trigger
	mu           sync.Mutex
	running      bool
	runID        string
	cancel       context.CancelFunc
	assessment   monitor.Assessment
	observations []monitor.Observation
	workers      sync.WaitGroup
	errors       chan error
}

func New(p monitor.Plan, profile diagnostics.Profile, s *transport.Server, store *storage.Store, path string) (*Coordinator, error) {
	if p.Diagnostics == nil || !p.Diagnostics.Enabled || p.Diagnostics.ProfileHash == "" || p.Diagnostics.ProfileRef != profile.Name {
		return nil, fmt.Errorf("resolved enabled diagnostics required")
	}
	if err := diagnostics.ValidateLiveProfile(profile); err != nil {
		return nil, err
	}
	t, e := diagnostics.CompileTrigger(p.Diagnostics.When)
	if e != nil {
		return nil, e
	}
	return &Coordinator{Plan: p, Profile: profile, Server: s, Store: store, ReportPath: path, trigger: t, errors: make(chan error, 1)}, nil
}
func (c *Coordinator) Tick(ctx context.Context, a monitor.Assessment, observations []monitor.Observation) error {
	select {
	case e := <-c.errors:
		return e
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.assessment = a
	c.observations = append([]monitor.Observation(nil), observations...)
	if c.running {
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
		if errors.Is(e, resources.ErrConflict) {
			return nil
		}
		return e
	}
	if id == "" {
		return nil
	}
	c.launch(ctx, id, c.Plan, c.Profile, agents, a, observations, started)
	return nil
}

// launch is called with mu held. Work belongs to the runtime, never the HTTP request.
func (c *Coordinator) launch(parent context.Context, id string, plan monitor.Plan, profile diagnostics.Profile, agents []diagnostics.Executor, a monitor.Assessment, observations []monitor.Observation, started time.Time) {
	ctx, cancel := context.WithCancel(parent)
	initial := diagnostics.MatchBaseline(plan, profile, observations, agents, time.Now())
	c.running = true
	c.runID = id
	c.cancel = cancel
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		defer cancel()
		defer func() {
			c.mu.Lock()
			if c.runID == id {
				c.running = false
				c.runID = ""
				c.cancel = nil
			}
			c.mu.Unlock()
		}()
		runner := transport.DiagnosticRunner{Server: c.Server, RunID: id, Plan: plan}
		engine := diagnostics.Engine{TaskOverhead: 20 * time.Second, Runner: runner, Eligible: func(action diagnostics.Action, agent diagnostics.Executor) bool {
			return agent.Supported["diagnostics:serial"] && (len(action.Check.Extract) == 0 || agent.Supported["extraction:regex"])
		}}
		mode := profile.Mode
		if plan.Diagnostics.ComparisonStatic {
			mode = "static"
		}
		result, e := engine.Run(ctx, profile, mode, agents, &diagnostics.Ledger{StartedAt: started}, initial, a.State)
		if e != nil {
			result.Status = "inconclusive"
			result.Reason = "planner_error"
		}
		persist, finish := context.WithTimeout(context.Background(), 5*time.Second)
		defer finish()
		if err := c.Store.FinishDiagnostic(persist, id, result); err != nil && !errors.Is(err, storage.ErrDiagnosticTerminal) {
			c.fail(err)
			return
		}
		if err := c.RestoreReport(persist); err != nil {
			c.fail(err)
		}
	}()
}

func (c *Coordinator) CancelRun(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runID == id && c.cancel != nil {
		c.cancel()
	}
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
