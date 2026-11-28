package agent

import (
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestResponseUsesCurrentAssignmentAndTypedLocalHelper(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil || request["port"] != float64(18088) || request["sourceCIDR"] != "188.137.254.31/32" {
			t.Error("incorrect helper request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"applied","reason":"kernel_timeout_installed"}`))
	})}
	go server.Serve(listener)
	defer server.Close()
	boot, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	st := &state{responseSocket: socket}
	st.set(&assignment{plan: monitor.Plan{Name: "web", Revision: "revision", Responses: []monitor.ResponseRule{{ID: "isolate", AgentRef: "branch", Port: 18088, SourceCIDR: "188.137.254.31/32", Duration: time.Minute}}}, deadline: time.Now().Add(20 * time.Second), bootDeadline: boot + 20*time.Second})
	command := &pb.ResponseCommand{Id: "0123456789abcdef0123456789abcdef", Revision: "revision", RuleId: "isolate", AcceptUntilUnixMilli: time.Now().Add(5 * time.Second).UnixMilli(), BlockUntilUnixMilli: time.Now().Add(time.Minute).UnixMilli()}
	if receipt := executeResponse(context.Background(), command, st, "branch"); receipt.Status != "applied" {
		t.Fatal(receipt)
	}
	command.Revision = "old"
	if receipt := executeResponse(context.Background(), command, st, "branch"); receipt.Status != "rejected" {
		t.Fatal(receipt)
	}
	command.Revision = "revision"
	command.AcceptUntilUnixMilli = time.Now().Add(-time.Second).UnixMilli()
	if receipt := executeResponse(context.Background(), command, st, "branch"); receipt.Status != "rejected" {
		t.Fatal(receipt)
	}
	if calls.Load() != 1 {
		t.Fatal("invalid commands reached helper", calls.Load())
	}
}
