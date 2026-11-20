package transport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
	"github.com/boby-star/NetMonOrc/internal/storage"
)

// RunFallbacks owns a bounded recovery lane; it shares the existing per-Agent
// one-shot admission slot with diagnostics, never the regular monitoring slot.
func (s *Server) RunFallbacks(ctx context.Context) error {
	started := time.Now()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var workers sync.WaitGroup
	defer workers.Wait()
	limit := make(chan struct{}, 2)
	active := map[string]bool{}
	tried := map[string]time.Time{}
	var mu sync.Mutex
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		cleanup, cancelCleanup := context.WithTimeout(ctx, 3*time.Second)
		err := s.Store.ExpireFallbacks(cleanup)
		cancelCleanup()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		mu.Lock()
		for key, at := range tried {
			if time.Since(at) > 2*time.Minute {
				delete(tried, key)
			}
		}
		mu.Unlock()
		scheduled := 0
		for _, p := range s.plans() {
			if p.Disabled || len(p.FallbackRoster) == 0 {
				continue
			}
			c, done := context.WithTimeout(ctx, 3*time.Second)
			history, err := s.Store.LatestRegular(c, p, time.Now())
			done()
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			bySource := map[string]monitor.Observation{}
			for _, o := range history {
				bySource[o.Agent+"/"+o.Check] = o
			}
			checks := append([]monitor.Check(nil), p.Checks...)
			sort.Slice(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
			for _, check := range checks {
				if check.Fallback == nil {
					continue
				}
				for _, source := range p.CheckSlots(check.ID, p.FallbackRoster) {
					if scheduled >= 64 {
						continue
					}
					now := time.Now()
					o, exists := bySource[source.Agent+"/"+check.ID]
					cause := monitor.FallbackCause(o)
					originID := o.ID
					at := o.FinishedAt
					if cause == "" {
						if s.sourceApplied(p, source.Agent) {
							continue
						}
						c, done := context.WithTimeout(ctx, time.Second)
						rejected, err := s.Store.ConfigRejected(c, source.Agent, p.Revision)
						done()
						if err != nil {
							return err
						}
						if rejected {
							cause = "config_rejected"
						} else {
							if now.Sub(started) < 30*time.Second || exists && now.Sub(o.FinishedAt) < check.Interval+check.Timeout+5*time.Second {
								continue
							}
							cause = "agent_unreachable"
						}
						originID = ""
						at = now.Truncate(check.Interval)
					}
					if !slices.Contains(check.Fallback.On, cause) {
						continue
					}
					key := p.Revision + "/" + source.Agent + "/" + check.ID + "/" + cause + "/" + originID
					if originID == "" {
						key += "/" + strconv.FormatInt(at.UnixNano(), 10)
					}
					sum := sha256.Sum256([]byte(key))
					key = hex.EncodeToString(sum[:])
					mu.Lock()
					busy := active[key] || time.Since(tried[key]) < 5*time.Second
					if !busy {
						select {
						case limit <- struct{}{}:
							active[key] = true
							tried[key] = time.Now()
						default:
							busy = true
						}
					}
					mu.Unlock()
					if busy {
						continue
					}
					r := storage.FallbackRun{Monitor: p.Name, Revision: p.Revision, Check: check.ID, OriginID: originID, OriginAgent: source.Agent, OriginZone: source.Zone, Cause: cause, Strategy: check.Fallback.Strategy, MaxAttempts: check.Fallback.MaxAttempts, StartedAt: at, Deadline: at.Add(p.MaxAge + 30*time.Second)}
					scheduled++
					workers.Add(1)
					go func() {
						defer workers.Done()
						defer func() { <-limit; mu.Lock(); delete(active, key); mu.Unlock() }()
						c, done := context.WithTimeout(ctx, 3*time.Second)
						run, err := s.Store.OpenFallback(c, key, r)
						done()
						if err != nil || run.State != "pending" || time.Now().After(run.Deadline) {
							return
						}
						s.executeFallback(ctx, p, check, run)
					}()
				}
			}
		}
	}
}
func (s *Server) sourceApplied(p monitor.Plan, agent string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.active[agent]
	return ok && v.appliedRevisions[p.Revision] && time.Since(v.lastHeartbeat) < 15*time.Second
}

func (s *Server) executeFallback(ctx context.Context, p monitor.Plan, check monitor.Check, run storage.FallbackRun) {
	c, done := context.WithTimeout(ctx, 3*time.Second)
	tasks, err := s.Store.FallbackTasks(c, run.ID)
	done()
	if err != nil {
		return
	}
	used := map[string]bool{}
	for _, task := range tasks {
		used[task.Agent] = true
		if (task.State == "reserved" || task.State == "accepted") && time.Now().Before(task.Deadline) {
			return
		}
	}
	for n := run.Used + 1; n <= run.MaxAttempts; n++ {
		var candidates []diagnostics.Executor
		for _, e := range s.Executors(time.Now()) {
			if used[e.ID] || !e.Supported["fallback:v1"] || !e.Supported[check.Runner] || !e.Supported[check.Type] || len(check.Extract) > 0 && !e.Supported["extraction:regex"] || check.NeedsHTTPPolicy() && !e.Supported["http:policy-v2"] {
				continue
			}
			var allowed []netip.Prefix
			for capability, supported := range e.Supported {
				if prefix, ok := strings.CutPrefix(capability, "target-cidr:"); ok && supported {
					if n, err := netip.ParsePrefix(prefix); err == nil {
						allowed = append(allowed, n)
					}
				}
			}
			if probe.CheckTargetPolicy(p, check, allowed) != nil {
				continue
			}
			if !p.FallbackCandidate(check, monitor.Slot{Agent: run.OriginAgent, Zone: run.OriginZone}, monitor.Slot{Agent: e.ID, Zone: e.Zone}, p.FallbackDomains) {
				continue
			}
			if check.Runner == "blackbox" && !s.ComponentReady(p, e.ID, time.Now()) {
				continue
			}
			candidates = append(candidates, e)
		}
		if len(candidates) == 0 {
			return
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
		candidate := candidates[0]
		used[candidate.ID] = true
		deadline := time.Now().Add(check.Timeout + 15*time.Second)
		if run.Deadline.Before(deadline) {
			deadline = run.Deadline
		}
		if time.Until(deadline) < check.Timeout+10*time.Second {
			return
		}
		attempt, cancel := context.WithDeadline(ctx, deadline)
		runner := DiagnosticRunner{Server: s, RunID: run.ID, Plan: p, Fallback: &run}
		action := diagnostics.Action{ID: check.ID, AttemptNo: n, Check: config.FallbackCheck(check)}
		outcome, err := runner.ExecuteEvidence(attempt, action, candidate)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil && outcome.Outcome.Execution == "completed" {
			c, done := context.WithTimeout(ctx, 3*time.Second)
			_ = s.Store.CloseFallback(c, run.ID, "completed")
			done()
			return
		}
		// Reload durable accounting: admission failure did not consume a task.
		c, done := context.WithTimeout(ctx, 3*time.Second)
		tasks, err = s.Store.FallbackTasks(c, run.ID)
		done()
		if err != nil || len(tasks) < n {
			return
		}
	}
	c, done = context.WithTimeout(ctx, 3*time.Second)
	_ = s.Store.CloseFallback(c, run.ID, "inconclusive")
	done()
}
