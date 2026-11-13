package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"os"
)

func TestSpoolRestartAndAck(t *testing.T) {
	dir := t.TempDir()
	s, e := OpenSpool(dir, "branch")
	if e != nil {
		t.Fatal(e)
	}
	o := monitor.Observation{ID: "0123456789abcdef0123456789abcdef", Agent: "branch", FinishedAt: time.Now().UTC()}
	if e = s.Add(o); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenSpool(dir, "branch"); e == nil {
		t.Fatal("concurrent spool writer accepted")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenSpool(dir, "branch")
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Pending()) != 1 || s.Pending()[0].ID != o.ID {
		t.Fatal("pending data lost on restart")
	}
	if e = s.Ack([]string{o.ID}); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenSpool(dir, "branch")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if len(s.Pending()) != 0 {
		t.Fatal("durably ACKed data reappeared")
	}
	if _, e = OpenSpool(dir, "other"); e == nil {
		t.Fatal("concurrent identity reused spool")
	}
}

func TestAssignmentLeaseAndChecksum(t *testing.T) {
	b, e := os.ReadFile("../../examples/http.yaml")
	if e != nil {
		t.Fatal(e)
	}
	p, e := config.Compile(b)
	if e != nil {
		t.Fatal(e)
	}
	b, e = json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	a := &pb.ConfigAssignment{CompiledPlan: b, Revision: p.Revision, Checksum: hex.EncodeToString(sum[:]), Zone: "office", ExpiresAtUnixMilli: time.Now().Add(30 * time.Second).UnixMilli()}
	s := &state{}
	start := time.Now()
	if e = apply(a, s, t.TempDir()); e != nil {
		t.Fatal(e)
	}
	if s.get().deadline.After(start.Add(25 * time.Second)) {
		t.Fatal("lease extended beyond uncertainty-adjusted deadline")
	}
	a.Checksum = "invalid"
	if e = apply(a, s, t.TempDir()); e == nil {
		t.Fatal("corrupt plan accepted")
	}
	a.Checksum = hex.EncodeToString(sum[:])
	a.ExpiresAtUnixMilli = time.Now().Add(-time.Second).UnixMilli()
	if e = apply(a, s, t.TempDir()); e == nil {
		t.Fatal("expired plan accepted")
	}
}

func TestExpiredLeaseDoesNotRunChecks(t *testing.T) {
	s, e := OpenSpool(t.TempDir(), "branch")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	st := &state{current: &assignment{plan: monitor.Plan{Revision: "test", Checks: []monitor.Check{{ID: "http", Runner: "native-http", URL: "http://127.0.0.1:1"}}}, zone: "office", deadline: time.Now().Add(-time.Second)}}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if e = schedule(ctx, st, s, "branch", "", nil); e != nil {
		t.Fatal(e)
	}
	if len(s.Pending()) != 0 {
		t.Fatal("new probe started after lease expired")
	}
}
