package transport

import (
	"fmt"
	"regexp"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

const Version = "v1alpha1"

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var observationID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var checkID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func Encode(o monitor.Observation) *pb.Observation {
	return &pb.Observation{DiagnosticTaskId: o.DiagnosticTaskID, Values: encodeValues(o.Values), ExtractionStatus: o.ExtractionStatus, Id: o.ID, Monitor: o.Monitor, Revision: o.Revision, Check: o.Check, Agent: o.Agent, Zone: o.Zone, StartedAt: o.StartedAt.Format(time.RFC3339Nano), FinishedAt: o.FinishedAt.Format(time.RFC3339Nano), Execution: o.Execution, Outcome: o.Outcome, Reason: o.Reason, StatusCode: int32(o.StatusCode)}
}

func Decode(w *pb.Observation, principal, zone, monitorName string) (monitor.Observation, error) {
	if w == nil || (w.DiagnosticTaskId != "" && !observationID.MatchString(w.DiagnosticTaskId)) || w.Agent != principal || w.Zone != zone || w.Zone == "" || len(w.Zone) > 63 || w.Monitor != monitorName || !observationID.MatchString(w.Id) || !revisionPattern.MatchString(w.Revision) || !checkID.MatchString(w.Check) || len(w.Reason) > 128 {
		return monitor.Observation{}, fmt.Errorf("invalid observation provenance or identity")
	}
	start, e := time.Parse(time.RFC3339Nano, w.StartedAt)
	if e != nil {
		return monitor.Observation{}, e
	}
	end, e := time.Parse(time.RFC3339Nano, w.FinishedAt)
	if e != nil || end.Before(start) || end.After(time.Now().Add(5*time.Second)) || start.IsZero() {
		return monitor.Observation{}, fmt.Errorf("invalid observation timestamps")
	}
	valid := (w.Execution == "completed" && (w.Outcome == "success" || w.Outcome == "failure")) || (w.Execution == "execution_failed" && w.Outcome == "unknown")
	if !valid || w.StatusCode < 0 || w.StatusCode > 599 {
		return monitor.Observation{}, fmt.Errorf("invalid observation outcome")
	}
	values, e := decodeValues(w)
	if e != nil {
		return monitor.Observation{}, e
	}
	return monitor.Observation{DiagnosticTaskID: w.DiagnosticTaskId, Values: values, ExtractionStatus: w.ExtractionStatus, ID: w.Id, Monitor: w.Monitor, Revision: w.Revision, Check: w.Check, Agent: principal, Zone: zone, StartedAt: start.UTC(), FinishedAt: end.UTC(), Execution: w.Execution, Outcome: w.Outcome, Reason: w.Reason, StatusCode: int(w.StatusCode)}, nil
}
