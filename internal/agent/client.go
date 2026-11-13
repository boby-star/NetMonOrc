package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	jitter "math/rand/v2"
	"net/netip"
	"path/filepath"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
	"github.com/boby-star/NetMonOrc/internal/transport"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type assignment struct {
	plan     monitor.Plan
	zone     string
	deadline time.Time
}
type state struct {
	mu      sync.Mutex
	current *assignment
}

func (s *state) get() *assignment  { s.mu.Lock(); defer s.mu.Unlock(); return s.current }
func (s *state) set(a *assignment) { s.mu.Lock(); defer s.mu.Unlock(); s.current = a }

func Run(ctx context.Context, endpoint string, tls *tls.Config, id, dir, blackbox string, allowed []netip.Prefix) error {
	spool, e := OpenSpool(dir, id)
	if e != nil {
		return e
	}
	defer spool.Close()
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	st := &state{}
	schedulerDone := make(chan error, 1)
	go func() { schedulerDone <- schedule(child, st, spool, id, blackbox, allowed) }()
	connectionDone := make(chan struct{})
	go func() {
		defer close(connectionDone)
		backoff := 250 * time.Millisecond
		for child.Err() == nil {
			start := time.Now()
			if err := connect(child, endpoint, tls, id, dir, spool, st); err != nil && child.Err() == nil {
				log.Print("Agent stream unavailable; retrying with pending observations retained")
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
	case e := <-schedulerDone:
		cancel()
		<-connectionDone
		return e
	}
}
func agentFrame(id string) *pb.AgentFrame {
	return &pb.AgentFrame{ProtocolVersion: transport.Version, AgentId: id, MessageId: rand.Text(), SentAtUnixMilli: time.Now().UnixMilli()}
}

func connect(ctx context.Context, endpoint string, tls *tls.Config, id, dir string, spool *Spool, st *state) error {
	conn, e := grpc.NewClient(endpoint, grpc.WithTransportCredentials(credentials.NewTLS(tls)), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1<<20), grpc.MaxCallSendMsgSize(1<<20)))
	if e != nil {
		return e
	}
	defer conn.Close()
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, e := pb.NewAgentServiceClient(conn).Connect(child)
	if e != nil {
		return e
	}
	f := agentFrame(id)
	f.Payload = &pb.AgentFrame_Hello{Hello: &pb.AgentHello{AgentVersion: "research-2", Capabilities: []string{"native-http", "blackbox", "http", "tcp", "dns", "icmp"}}}
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
	var flight map[string]bool
	var sent time.Time
	for {
		select {
		case <-child.Done():
			return child.Err()
		case <-tick.C:
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
				batch.Observations = append(batch.Observations, transport.Encode(o))
				flight[o.ID] = true
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
				if e = apply(a, st, dir); e != nil {
					return e
				}
				f := agentFrame(id)
				f.Payload = &pb.AgentFrame_ConfigApplied{ConfigApplied: &pb.ConfigApplied{AssignmentId: a.AssignmentId, Revision: a.Revision}}
				if e = stream.Send(f); e != nil {
					return e
				}
				applied = true
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
func apply(a *pb.ConfigAssignment, st *state, dir string) error {
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
	if e := config.VerifyPlan(p, a.Zone); e != nil {
		return e
	}
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	if e = durable.AtomicWrite(filepath.Join(dir, ".assignment"), b); e != nil {
		return e
	}
	// time.Now().Add retains a monotonic deadline for the running process.
	// Saved assignments are never resumed after process/OS restart.
	st.set(&assignment{p, a.Zone, now.Add(remaining)})
	return nil
}

func schedule(ctx context.Context, st *state, spool *Spool, id, blackbox string, allowed []netip.Prefix) error {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	next := map[string]time.Time{}
	revision := ""
	var exporter *probe.Exporter
	defer func() {
		if exporter != nil {
			exporter.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		a := st.get()
		if a == nil || !time.Now().Before(a.deadline) || spool.Full() {
			continue
		}
		if revision != a.plan.Revision {
			next = map[string]time.Time{}
			revision = a.plan.Revision
			if exporter != nil {
				exporter.Close()
				exporter = nil
			}
		}
		for _, c := range a.plan.Checks {
			if ctx.Err() != nil {
				return nil
			}
			if spool.Full() || !time.Now().Before(a.deadline) {
				break
			}
			if time.Now().Before(next[c.ID]) {
				continue
			}
			next[c.ID] = time.Now().Add(c.Interval)
			child, cancel := context.WithDeadline(ctx, a.deadline)
			slot := monitor.Slot{Agent: id, Zone: a.zone}
			var o monitor.Observation
			var e error
			if c.Runner == "blackbox" {
				if exporter == nil {
					exporter, e = probe.StartExporter(ctx, blackbox, a.plan)
				}
				if e == nil {
					o, e = exporter.Run(child, a.plan, c, slot, allowed)
				}
			} else {
				o, e = probe.HTTP(child, a.plan, c, slot, allowed)
			}
			cancel()
			if e != nil {
				var id [16]byte
				if _, err := rand.Read(id[:]); err != nil {
					return err
				}
				now := time.Now().UTC()
				o = monitor.Observation{ID: hex.EncodeToString(id[:]), Monitor: a.plan.Name, Revision: a.plan.Revision, Check: c.ID, Agent: slot.Agent, Zone: slot.Zone, StartedAt: now, FinishedAt: now, Execution: "execution_failed", Outcome: "unknown", Reason: "runner_unavailable_or_policy_denied"}
			}
			if e = spool.Add(o); e != nil {
				return e
			}
		}
	}
}
