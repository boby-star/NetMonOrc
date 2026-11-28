package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	jitter "math/rand/v2"
	"net/netip"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boby-star/NetMonOrc/internal/component"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
	"github.com/boby-star/NetMonOrc/internal/transport"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type assignment struct {
	plan         monitor.Plan
	zone         string
	deadline     time.Time
	bootDeadline time.Duration
}
type state struct {
	responseSocket string
	agentID        string
	components     map[string]component.Readiness
	mu             sync.Mutex
	current        *assignment
	assignments    map[string]*assignment
	active         atomic.Int32
	diagnosticBusy atomic.Bool
	fatal          chan error
}

func (s *state) get() *assignment { s.mu.Lock(); defer s.mu.Unlock(); return s.current }
func (s *state) set(a *assignment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assignments == nil {
		s.assignments = map[string]*assignment{}
	}
	s.assignments[a.plan.Name] = a
	s.current = a
}
func (s *state) list() []*assignment {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.assignments) == 0 {
		if s.current != nil {
			return []*assignment{s.current}
		}
		return nil
	}
	out := make([]*assignment, 0, len(s.assignments))
	for name, a := range s.assignments {
		if leaseValid(a) {
			out = append(out, a)
		} else {
			delete(s.assignments, name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].plan.Name < out[j].plan.Name })
	return out
}
func (s *state) forRevision(revision string) *assignment {
	for _, a := range s.list() {
		if a.plan.Revision == revision {
			return a
		}
	}
	return nil
}

func Run(ctx context.Context, endpoint string, tls *tls.Config, id, dir, blackbox string, allowed []netip.Prefix, responseSocket ...string) error {
	spool, e := OpenSpool(dir, id)
	if e != nil {
		return e
	}
	defer spool.Close()
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	st := &state{agentID: id, fatal: make(chan error, 1)}
	if len(responseSocket) > 0 {
		st.responseSocket = responseSocket[0]
	}
	schedulerDone := make(chan error, 1)
	go func() { schedulerDone <- schedule(child, st, spool, id, blackbox, allowed) }()
	componentDone := make(chan struct{})
	go func() { defer close(componentDone); monitorComponents(child, st, id) }()
	defer func() { cancel(); <-componentDone }()
	connectionDone := make(chan struct{})
	go func() {
		defer close(connectionDone)
		backoff := 250 * time.Millisecond
		for child.Err() == nil {
			start := time.Now()
			if err := connect(child, endpoint, tls, id, dir, spool, st, blackbox, allowed); err != nil && child.Err() == nil {
				log.Printf("Agent stream unavailable (%s): %.512s; pending observations retained", status.Code(err), status.Convert(err).Message())
			}
			if time.Since(start) > 30*time.Second {
				backoff = 250 * time.Millisecond
			}
			delay := backoff + time.Duration(jitter.IntN(100))*time.Millisecond
			t := time.NewTimer(delay)
			select {
			case <-child.Done():
				t.Stop()
				return
			case <-t.C:
			}
			backoff = min(backoff*2, 5*time.Second)
		}
	}()
	select {
	case <-ctx.Done():
		cancel()
		<-schedulerDone
		<-connectionDone
		return nil
	case e := <-st.fatal:
		cancel()
		<-schedulerDone
		<-connectionDone
		return e
	case e := <-schedulerDone:
		cancel()
		<-connectionDone
		return e
	}
}
func agentFrame(id string) *pb.AgentFrame {
	return &pb.AgentFrame{ProtocolVersion: transport.Version, AgentId: id, MessageId: rand.Text(), SentAtUnixMilli: time.Now().UnixMilli()}
}

func connect(ctx context.Context, endpoint string, tls *tls.Config, id, dir string, spool *Spool, st *state, blackbox string, allowed []netip.Prefix) error {
	conn, e := grpc.NewClient(endpoint, grpc.WithNoProxy(), grpc.WithTransportCredentials(credentials.NewTLS(tls)), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1<<20), grpc.MaxCallSendMsgSize(1<<20)))
	if e != nil {
		return e
	}
	defer conn.Close()
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	var taskID string
	var taskCancel context.CancelFunc
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	stream, e := pb.NewAgentServiceClient(conn).Connect(child)
	if e != nil {
		return e
	}
	f := agentFrame(id)
	f.Payload = &pb.AgentFrame_Hello{Hello: &pb.AgentHello{AgentVersion: "research-5", Capabilities: []string{"fallback:v1", "components:readiness-v1", "components:external-v1", "native-http", "blackbox", "http", "tcp", "dns", "icmp", "extraction:regex", "diagnostics:serial", "placement:fixed", "placement:dynamic", "placement:domains", "placement:per-check", "http:policy-v2", "assessment:zone-any", "assessment:replicas", "context:dependencies", "plans:multiple", "yaml:controls-v1", "diagnostics:cancel", "diagnostics:static-comparison"}}}
	for _, cidr := range allowed {
		f.GetHello().Capabilities = append(f.GetHello().Capabilities, "target-cidr:"+cidr.Masked().String())
	}
	if st.responseSocket != "" {
		f.GetHello().Capabilities = append(f.GetHello().Capabilities, "response:block-port-v1")
	}
	f.GetHello().Capabilities = append(f.GetHello().Capabilities, "response:policy-v1")
	if e = stream.Send(f); e != nil {
		return e
	}
	type received struct {
		f *pb.ControllerFrame
		e error
	}
	incoming := make(chan received, 1)
	go func() {
		for {
			f, e := stream.Recv()
			select {
			case incoming <- received{f, e}:
			case <-child.Done():
				return
			}
			if e != nil {
				return
			}
		}
	}()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	lastReceived := time.Now()
	accepted := false
	applied := false
	appliedRevisions := map[string]bool{}
	var flight map[string]bool
	var sent time.Time
	var sequence uint64
	lastHeartbeat := time.Time{}
	for {
		select {
		case <-child.Done():
			return child.Err()
		case <-tick.C:
			if applied && time.Since(lastHeartbeat) >= 5*time.Second {
				sequence++
				f := agentFrame(id)
				f.Payload = &pb.AgentFrame_Heartbeat{Heartbeat: &pb.Heartbeat{Sequence: sequence, MaxConcurrentProbes: 2, ActiveProbes: uint32(st.active.Load()), Components: componentReports(st, id, appliedRevisions)}}
				if e = stream.Send(f); e != nil {
					return e
				}
				lastHeartbeat = time.Now()
				if e = cleanupTaskReceipts(dir); e != nil {
					return e
				}
			}
			if time.Since(lastReceived) > 20*time.Second {
				return fmt.Errorf("Controller lease renewal timeout")
			}
			if len(flight) > 0 {
				if time.Since(sent) > 10*time.Second {
					return fmt.Errorf("observation ACK timeout")
				}
				continue
			}
			if !applied {
				continue
			}
			pending := spool.Pending()
			if len(pending) == 0 {
				continue
			}
			batch := &pb.ObservationBatch{}
			flight = map[string]bool{}
			for _, o := range pending {
				if st.forRevision(o.Revision) != nil && !appliedRevisions[o.Revision] {
					continue
				}
				batch.Observations = append(batch.Observations, transport.Encode(o))
				flight[o.ID] = true
			}
			if len(batch.Observations) == 0 {
				continue
			}
			f := agentFrame(id)
			f.Payload = &pb.AgentFrame_ObservationBatch{ObservationBatch: batch}
			if e = stream.Send(f); e != nil {
				return e
			}
			sent = time.Now()
		case r := <-incoming:
			if r.e != nil {
				return r.e
			}
			f := r.f
			if f == nil || f.ProtocolVersion != transport.Version || f.AgentId != id || f.MessageId == "" {
				return fmt.Errorf("invalid Controller envelope")
			}
			age := time.Now().UnixMilli() - f.SentAtUnixMilli
			if age < -5000 || age > 5000 {
				return fmt.Errorf("Controller clock or delivery outside supported uncertainty")
			}
			lastReceived = time.Now()
			if hello := f.GetHelloAccepted(); hello != nil {
				if accepted || hello.SessionGeneration < 1 {
					return fmt.Errorf("invalid session generation")
				}
				accepted = true
				continue
			}
			if a := f.GetAssignment(); a != nil {
				if !accepted {
					return fmt.Errorf("assignment before session acceptance")
				}
				if e = apply(a, st, dir, id); e != nil {
					reject := agentFrame(id)
					reject.Payload = &pb.AgentFrame_ConfigRejected{ConfigRejected: &pb.ConfigRejected{AssignmentId: a.AssignmentId, Revision: a.Revision}}
					_ = stream.Send(reject)
					return e
				}
				f := agentFrame(id)
				f.Payload = &pb.AgentFrame_ConfigApplied{ConfigApplied: &pb.ConfigApplied{AssignmentId: a.AssignmentId, Revision: a.Revision}}
				if e = stream.Send(f); e != nil {
					return e
				}
				applied = true
				appliedRevisions[a.Revision] = true
				continue
			}
			if command := f.GetResponseCommand(); command != nil {
				receipt := &pb.ResponseReceipt{Id: command.Id, Status: "rejected", Reason: "assignment_not_applied"}
				if accepted && appliedRevisions[command.Revision] {
					receipt = executeResponse(child, command, st, id)
				}
				reply := agentFrame(id)
				reply.Payload = &pb.AgentFrame_ResponseReceipt{ResponseReceipt: receipt}
				if e = stream.Send(reply); e != nil {
					return e
				}
				continue
			}
			if message := f.GetTaskCancel(); message != nil {
				if message.TaskId == taskID && taskCancel != nil {
					taskCancel()
				}
				continue
			}
			if task := f.GetDiagnosticTask(); task != nil {
				receipt := &pb.TaskReceipt{TaskId: task.TaskId, Reason: "invalid_or_busy"}
				var a *assignment
				var c monitor.Check
				var deadline time.Time
				reserved := accepted && applied && appliedRevisions[task.Revision] && st.diagnosticBusy.CompareAndSwap(false, true)
				if reserved && !spool.Reserve() {
					st.diagnosticBusy.Store(false)
					reserved = false
				}
				if reserved {
					a, c, deadline, e = prepareTask(task, st, dir)
					receipt.Accepted = e == nil
					if receipt.Accepted {
						receipt.Reason = ""
					}
					if !receipt.Accepted {
						st.diagnosticBusy.Store(false)
						spool.Release()
					}
				}
				reply := agentFrame(id)
				reply.Payload = &pb.AgentFrame_TaskReceipt{TaskReceipt: receipt}
				if e = stream.Send(reply); e != nil {
					if receipt.Accepted {
						st.diagnosticBusy.Store(false)
						spool.Release()
					}
					return e
				}
				if receipt.Accepted {
					taskContext, stopTask := context.WithCancel(child)
					taskID = task.TaskId
					taskCancel = stopTask
					workers.Add(1)
					go func() {
						defer workers.Done()
						defer stopTask()
						if err := executeTask(taskContext, task, a, c, deadline, st, spool, id, blackbox, allowed); err != nil {
							select {
							case st.fatal <- err:
							default:
							}
						}
					}()
				}
				continue
			}
			if ack := f.GetObservationAck(); ack != nil {
				if len(flight) == 0 || len(ack.Receipts) != len(flight) {
					return fmt.Errorf("unexpected ACK")
				}
				var ids []string
				for _, receipt := range ack.Receipts {
					if !flight[receipt.ObservationId] || (receipt.Status != pb.ObservationReceipt_ACCEPTED && receipt.Status != pb.ObservationReceipt_DUPLICATE) {
						return fmt.Errorf("invalid durable receipt")
					}
					delete(flight, receipt.ObservationId)
					ids = append(ids, receipt.ObservationId)
				}
				if e = spool.Ack(ids); e != nil {
					return e
				}
				flight = nil
				continue
			}
			return fmt.Errorf("unsupported Controller frame")
		}
	}
}
func apply(a *pb.ConfigAssignment, st *state, dir string, agent ...string) error {
	boot, err := bootTime()
	if err != nil {
		return err
	}
	now := time.Now()
	remaining := time.UnixMilli(a.ExpiresAtUnixMilli).Sub(now) - 5*time.Second
	if remaining <= 0 || remaining > 30*time.Second {
		return fmt.Errorf("invalid assignment lease")
	}
	sum := sha256.Sum256(a.CompiledPlan)
	if hex.EncodeToString(sum[:]) != a.Checksum {
		return fmt.Errorf("assignment checksum mismatch")
	}
	var p monitor.Plan
	d := json.NewDecoder(bytes.NewReader(a.CompiledPlan))
	d.DisallowUnknownFields()
	if e := d.Decode(&p); e != nil {
		return e
	}
	if p.Revision != a.Revision {
		return fmt.Errorf("assignment revision mismatch")
	}
	if e := config.VerifyPlan(p, a.Zone, agent...); e != nil {
		return e
	}
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	if e = durable.AtomicWrite(filepath.Join(dir, ".assignment-"+p.Name), b); e != nil {
		return e
	}
	// time.Now().Add retains a monotonic deadline for the running process.
	// Saved assignments are never resumed after process/OS restart.
	st.set(&assignment{plan: p, zone: a.Zone, deadline: now.Add(remaining), bootDeadline: boot + remaining})
	return nil
}

func schedule(ctx context.Context, st *state, spool *Spool, id, blackbox string, allowed []netip.Prefix) error {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	next := map[string]time.Time{}
	attempts := map[string]uint64{}
	type cached struct {
		exporter *probe.Exporter
		used     time.Time
	}
	exporters := map[string]cached{}
	turn := 0
	defer func() {
		for _, v := range exporters {
			v.exporter.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		assignments := st.list()
		if len(assignments) > 0 {
			start := turn % len(assignments)
			assignments = append(assignments[start:], assignments[:start]...)
			turn = (start + 1) % len(assignments)
		}
		live := map[string]bool{}
		for _, a := range assignments {
			if leaseValid(a) {
				live[a.plan.Revision] = true
			}
		}
		for revision, v := range exporters {
			if !live[revision] {
				v.exporter.Close()
				delete(exporters, revision)
			}
		}
		for key := range next {
			revision, _, _ := strings.Cut(key, "/")
			if !live[revision] {
				delete(next, key)
				delete(attempts, key)
			}
		}
		for _, job := range dueRegular(assignments, id, next, time.Now()) {
			a, c := job.assignment, job.check
			{
				if ctx.Err() != nil {
					return nil
				}
				if spool.Full() {
					break
				}
				if !leaseValid(a) {
					continue
				}
				key := a.plan.Revision + "/" + c.ID
				if time.Now().Before(next[key]) {
					continue
				}
				if !spool.Reserve() {
					break
				}
				next[key] = time.Now().Add(c.Interval + scheduleJitter(id, key, attempts[key]+1, c.Jitter))
				attempts[key]++
				child, cancel := context.WithDeadline(ctx, a.deadline)
				slot := monitor.Slot{Agent: id, Zone: a.zone}
				var o monitor.Observation
				var e error
				st.active.Add(1)
				if c.Runner == "blackbox" {
					entry, found := exporters[a.plan.Revision]
					if !found {
						// Two regular exporters bound memory independently of catalog size.
						if len(exporters) >= 2 {
							oldest := ""
							for rev, v := range exporters {
								if oldest == "" || v.used.Before(exporters[oldest].used) {
									oldest = rev
								}
							}
							exporters[oldest].exporter.Close()
							delete(exporters, oldest)
						}
						entry.exporter, e = probe.StartAgentExporter(ctx, blackbox, a.plan, id)
					}
					if e == nil {
						entry.used = time.Now()
						exporters[a.plan.Revision] = entry
						if !leaseValid(a) {
							e = fmt.Errorf("lease expired during exporter startup")
						} else {
							o, e = entry.exporter.Run(child, a.plan, c, slot, allowed)
						}
					}
				} else {
					o, e = probe.HTTP(child, a.plan, c, slot, allowed)
				}
				cancel()
				st.active.Add(-1)
				if e != nil {
					var observationID [16]byte
					if _, err := rand.Read(observationID[:]); err != nil {
						spool.Release()
						return err
					}
					now := time.Now().UTC()
					o = monitor.Observation{ID: hex.EncodeToString(observationID[:]), Monitor: a.plan.Name, Revision: a.plan.Revision, Check: c.ID, Agent: slot.Agent, Zone: slot.Zone, StartedAt: now, FinishedAt: now, Execution: "execution_failed", Outcome: "unknown", Reason: "local_execution_error"}
					if probe.CheckTargetPolicy(a.plan, c, allowed) != nil {
						o.Reason = "target_policy_denied"
					} else if c.Runner == "blackbox" {
						o.Reason = "runner_unavailable"
					}
				}
				if errors.Is(e, probe.ErrRunnerUnavailable) {
					o.Reason = "runner_unavailable"
				}
				if e = spool.AddReserved(o); e != nil {
					return e
				}
			}
		}
	}
}
