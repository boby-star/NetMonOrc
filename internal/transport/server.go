package transport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/secure"
	"github.com/boby-star/NetMonOrc/internal/storage"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type activeSession struct {
	certExpiresAt     time.Time
	generation        int64
	cancel            context.CancelFunc
	tasks             chan taskRequest
	applied           bool
	appliedRevisions  map[string]bool
	capabilities      map[string]bool
	lastHeartbeat     time.Time
	capacity          int
	active            int
	heartbeatSequence uint64
}
type Server struct {
	// Immutable Controller-owned domains, configured before serving.
	FailureDomains        map[string]string
	FailureDomainsKnownAt time.Time
	pb.UnimplementedAgentServiceServer
	Plan                 monitor.Plan
	Plans                []monitor.Plan
	Slots                []monitor.Slot
	Store                *storage.Store
	mu                   sync.Mutex
	active               map[string]activeSession
	tasks                map[string]taskWaiter
	diagnosticAdmissions map[string]*diagnosticAdmission
	admissionChanged     chan struct{}
}

func frame(agent string) *pb.ControllerFrame {
	return &pb.ControllerFrame{ProtocolVersion: Version, AgentId: agent, MessageId: rand.Text(), SentAtUnixMilli: time.Now().UnixMilli()}
}
func (s *Server) Connect(stream pb.AgentService_ConnectServer) error {
	p, ok := peer.FromContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing TLS peer")
	}
	auth, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(auth.State.VerifiedChains) == 0 || len(auth.State.PeerCertificates) == 0 {
		return status.Error(codes.Unauthenticated, "verified client certificate required")
	}
	id, e := secure.Principal(auth.State.PeerCertificates[0])
	if e != nil {
		return status.Error(codes.PermissionDenied, "invalid Agent certificate identity")
	}
	zone := ""
	for _, slot := range s.Slots {
		if slot.Agent == id {
			zone = slot.Zone
		}
	}
	if zone == "" {
		return status.Error(codes.PermissionDenied, "Agent is not in Controller roster")
	}
	ctx, cancel := context.WithDeadline(stream.Context(), auth.State.PeerCertificates[0].NotAfter)
	defer cancel()
	type received struct {
		f *pb.AgentFrame
		e error
	}
	incoming := make(chan received, 1)
	go func() {
		for {
			f, e := stream.Recv()
			select {
			case incoming <- received{f, e}:
			case <-ctx.Done():
				return
			}
			if e != nil {
				return
			}
		}
	}()
	firstTimeout := time.NewTimer(5 * time.Second)
	defer firstTimeout.Stop()
	var hello *pb.AgentFrame
	select {
	case r := <-incoming:
		if r.e != nil {
			return r.e
		}
		hello = r.f
	case <-firstTimeout.C:
		return status.Error(codes.DeadlineExceeded, "Hello timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
	if !validFrame(hello, id) || hello.GetHello() == nil {
		return status.Error(codes.InvalidArgument, "expected compatible AgentHello")
	}
	capabilities := map[string]bool{}
	for _, c := range hello.GetHello().Capabilities {
		capabilities[c] = true
	}
	if len(s.plans()) > 1 && !capabilities["plans:multiple"] {
		return status.Error(codes.FailedPrecondition, "multiple plan capability required")
	}
	for _, plan := range s.plans() {
		if (plan.SameZoneAggregation != "" || len(plan.ReplicaSources()) > 1 || (plan.FixedPlacement != nil && plan.FixedPlacement.DistinctBy != "")) && !capabilities["assessment:replicas"] {
			return status.Error(codes.FailedPrecondition, "replica assessment capability required")
		}
		if len(plan.Dependencies) > 0 && !capabilities["context:dependencies"] {
			return status.Error(codes.FailedPrecondition, "dependency context capability required")
		}
		if plan.ZoneAggregation == "any" && !capabilities["assessment:zone-any"] {
			return status.Error(codes.FailedPrecondition, "zone any assessment capability required")
		}
		if plan.FixedPlacement != nil && !capabilities["placement:fixed"] {
			return status.Error(codes.FailedPrecondition, "fixed placement capability required")
		}
		if plan.FixedPlacement != nil && plan.FixedPlacement.Strategy != "" && !capabilities["placement:dynamic"] {
			return status.Error(codes.FailedPrecondition, "dynamic placement capability required")
		}
		for _, c := range plan.Checks {
			if !capabilities[c.Runner] || !capabilities[c.Type] || (len(c.Extract) > 0 && !capabilities["extraction:regex"]) {
				return status.Error(codes.FailedPrecondition, "runner capability unavailable")
			}
		}
		if plan.Diagnostics != nil && plan.Diagnostics.Enabled && !capabilities["diagnostics:serial"] {
			return status.Error(codes.FailedPrecondition, "diagnostic execution capability unavailable")
		}
		if plan.Diagnostics != nil && plan.Diagnostics.ComparisonStatic && !capabilities["diagnostics:static-comparison"] {
			return status.Error(codes.FailedPrecondition, "static comparison IR capability unavailable")
		}
	}
	dbctx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
	generation, e := s.Store.BeginSession(dbctx, id)
	dbCancel()
	if e != nil {
		return status.Error(codes.Unavailable, "cannot establish durable session")
	}
	s.mu.Lock()
	if s.active == nil {
		s.active = map[string]activeSession{}
	}
	old, exists := s.active[id]
	if exists && old.generation >= generation {
		s.mu.Unlock()
		return status.Error(codes.Aborted, "session superseded")
	}
	if exists {
		old.cancel()
	}
	jobs := make(chan taskRequest, 1)
	s.active[id] = activeSession{generation: generation, cancel: cancel, tasks: jobs, appliedRevisions: map[string]bool{}, capabilities: capabilities, certExpiresAt: auth.State.PeerCertificates[0].NotAfter}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.active[id].generation == generation {
			delete(s.active, id)
		}
		s.mu.Unlock()
	}()
	f := frame(id)
	f.Payload = &pb.ControllerFrame_HelloAccepted{HelloAccepted: &pb.HelloAccepted{SessionGeneration: generation}}
	if e = stream.Send(f); e != nil {
		return e
	}
	plans := s.plans()
	assignments := map[string]monitor.Plan{}
	for _, plan := range plans {
		assignments[fmt.Sprintf("%s-%d", plan.Revision, generation)] = plan
	}
	sendAssignment := func() error {
		for _, p := range plans {
			plan, e := json.Marshal(p)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(plan)
			c, done := context.WithTimeout(ctx, 5*time.Second)
			assignmentErr := s.Store.Assign(c, id, generation, p.Revision, zone, p.Name, p.RegularAllowed(id))
			done()
			if assignmentErr != nil {
				return status.Error(codes.Aborted, "session superseded or storage unavailable")
			}
			f := frame(id)
			f.Payload = &pb.ControllerFrame_Assignment{Assignment: &pb.ConfigAssignment{AssignmentId: fmt.Sprintf("%s-%d", p.Revision, generation), CompiledPlan: plan, Revision: p.Revision, Checksum: hex.EncodeToString(sum[:]), ExpiresAtUnixMilli: min(time.Now().Add(30*time.Second).UnixMilli(), auth.State.PeerCertificates[0].NotAfter.UnixMilli()), Zone: zone}}
			if e = stream.Send(f); e != nil {
				return e
			}
		}
		return nil
	}
	if e = sendAssignment(); e != nil {
		return e
	}
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	applied := map[string]bool{}
	appliedRevision := ""
	for {
		select {
		case <-ctx.Done():
			return status.Error(codes.Aborted, "session ended or superseded")
		case req := <-jobs:
			if req.cancel != nil {
				f := frame(id)
				f.Payload = &pb.ControllerFrame_TaskCancel{TaskCancel: req.cancel}
				if e = stream.Send(f); e != nil {
					return e
				}
				continue
			}
			if req.ctx.Err() != nil {
				continue
			}
			if !applied[req.task.Revision] {
				s.taskResult(req.task.TaskId, id, diagnostics.Outcome{Execution: "execution_failed", Outcome: "unknown"})
				continue
			}
			// Refresh authorization before a potentially long physical probe.
			// This extends no in-flight task and never survives disconnect.
			if e = sendAssignment(); e != nil {
				return e
			}
			if req.ctx.Err() != nil {
				continue
			}
			f := frame(id)
			f.Payload = &pb.ControllerFrame_DiagnosticTask{DiagnosticTask: req.task}
			if e = stream.Send(f); e != nil {
				return e
			}
		case <-tick.C:
			if e = sendAssignment(); e != nil {
				return e
			}
		case r := <-incoming:
			if r.e == io.EOF {
				return nil
			}
			if r.e != nil {
				return r.e
			}
			if !validFrame(r.f, id) {
				return status.Error(codes.InvalidArgument, "invalid Agent envelope")
			}
			if a := r.f.GetConfigApplied(); a != nil {
				plan, known := assignments[a.AssignmentId]
				if !known || a.Revision != plan.Revision {
					return status.Error(codes.InvalidArgument, "unexpected applied revision")
				}
				c, done := context.WithTimeout(ctx, 5*time.Second)
				e = s.Store.Applied(c, id, generation, a.Revision)
				done()
				if e != nil {
					return status.Error(codes.Aborted, "cannot apply fenced session")
				}
				s.mu.Lock()
				v := s.active[id]
				if v.generation == generation {
					v.appliedRevisions[a.Revision] = true
					v.applied = len(v.appliedRevisions) == len(plans)
					s.active[id] = v
				}
				s.mu.Unlock()
				applied[a.Revision] = true
				appliedRevision = a.Revision
				continue
			}
			if h := r.f.GetHeartbeat(); h != nil {
				if len(applied) == 0 || h.Sequence == 0 || h.MaxConcurrentProbes != 2 || h.ActiveProbes > h.MaxConcurrentProbes {
					return status.Error(codes.InvalidArgument, "invalid heartbeat")
				}
				s.mu.Lock()
				v := s.active[id]
				valid := v.generation == generation && h.Sequence > v.heartbeatSequence
				if valid {
					v.lastHeartbeat = time.Now()
					v.heartbeatSequence = h.Sequence
					v.capacity = int(h.MaxConcurrentProbes)
					v.active = int(h.ActiveProbes)
					s.active[id] = v
				}
				s.mu.Unlock()
				if !valid {
					return status.Error(codes.Aborted, "stale heartbeat generation or sequence")
				}
				continue
			}
			if receipt := r.f.GetTaskReceipt(); receipt != nil {
				if len(applied) == 0 || !observationID.MatchString(receipt.TaskId) || len(receipt.Reason) > 128 {
					return status.Error(codes.InvalidArgument, "invalid task receipt")
				}
				c, done := context.WithTimeout(ctx, 5*time.Second)
				err := s.Store.DiagnosticReceipt(c, receipt.TaskId, id, generation, receipt.Accepted)
				done()
				if err != nil {
					return status.Error(codes.Aborted, "task receipt could not be fenced")
				}
				if !receipt.Accepted {
					s.taskResult(receipt.TaskId, id, diagnostics.Outcome{Execution: "execution_failed", Outcome: "unknown"})
				}
				continue
			}
			batch := r.f.GetObservationBatch()
			if len(applied) == 0 || batch == nil || len(batch.Observations) < 1 || len(batch.Observations) > 32 {
				return status.Error(codes.InvalidArgument, "invalid observation batch")
			}
			observations := make([]monitor.Observation, 0, len(batch.Observations))
			seen := map[string]bool{}
			for _, w := range batch.Observations {
				// The DB verifies this zone against the historical assignment.
				o, e := Decode(w, id, w.GetZone(), w.GetMonitor())
				if e != nil || seen[w.GetId()] {
					return status.Error(codes.InvalidArgument, "invalid or conflicting observation")
				}
				if plan, known := s.planForRevision(o.Revision); known {
					if !applied[o.Revision] {
						return status.Error(codes.FailedPrecondition, "observation plan is not acknowledged")
					}
					if o.DiagnosticTaskID == "" && !plan.RegularAllowed(id) {
						return status.Error(codes.PermissionDenied, "Agent is not assigned regular probes")
					}
				}
				seen[o.ID] = true
				observations = append(observations, o)
			}
			c, done := context.WithTimeout(ctx, 5*time.Second)
			duplicates, e := s.Store.CommitBatch(c, id, generation, appliedRevision, observations)
			done()
			if e != nil {
				return status.Error(codes.Unavailable, "durable observation commit failed")
			}
			for _, o := range observations {
				if o.DiagnosticTaskID != "" {
					s.taskEvidenceResult(o.DiagnosticTaskID, id, diagnostics.ObservedOutcome{ObservationID: o.ID, FinishedAt: o.FinishedAt, Outcome: diagnostics.Outcome{Execution: o.Execution, Outcome: o.Outcome}})
				}
			}
			ack := &pb.ObservationAck{}
			for i, o := range observations {
				st := pb.ObservationReceipt_ACCEPTED
				if duplicates[i] {
					st = pb.ObservationReceipt_DUPLICATE
				}
				ack.Receipts = append(ack.Receipts, &pb.ObservationReceipt{ObservationId: o.ID, Status: st})
			}
			f := frame(id)
			f.Payload = &pb.ControllerFrame_ObservationAck{ObservationAck: ack}
			if e = stream.Send(f); e != nil {
				return e
			}
		}
	}
}
func validFrame(f *pb.AgentFrame, id string) bool {
	return f != nil && f.ProtocolVersion == Version && f.AgentId == id && f.MessageId != "" && len(f.MessageId) <= 128 && f.SentAtUnixMilli > 0 && f.SentAtUnixMilli <= time.Now().Add(5*time.Second).UnixMilli()
}

// plans is immutable for the lifetime of a Controller stream.
func (s *Server) plans() []monitor.Plan {
	if len(s.Plans) > 0 {
		return s.Plans
	}
	return []monitor.Plan{s.Plan}
}
func (s *Server) planForRevision(revision string) (monitor.Plan, bool) {
	for _, p := range s.plans() {
		if p.Revision == revision {
			return p, true
		}
	}
	return monitor.Plan{}, false
}
