package agent

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

func executeResponse(ctx context.Context, command *pb.ResponseCommand, st *state, id string) *pb.ResponseReceipt {
	result := &pb.ResponseReceipt{Id: command.Id, Status: "rejected", Reason: "invalid_or_expired_response"}
	key, err := hex.DecodeString(command.Id)
	now := time.Now()
	if err != nil || len(key) != 16 || st.responseSocket == "" || command.AcceptUntilUnixMilli <= now.UnixMilli() || command.AcceptUntilUnixMilli > now.Add(15*time.Second).UnixMilli() {
		return result
	}
	a := st.forRevision(command.Revision)
	if a == nil || !leaseValid(a) || a.plan.Disabled {
		return result
	}
	for _, r := range a.plan.Responses {
		if r.ID != command.RuleId || r.AgentRef != id {
			continue
		}
		until := time.UnixMilli(command.BlockUntilUnixMilli)
		if !until.After(now) || until.After(now.Add(r.Duration+time.Second)) {
			return result
		}
		payload, err := json.Marshal(map[string]any{"id": command.Id, "port": r.Port, "sourceCIDR": r.SourceCIDR, "untilUnixMilli": command.BlockUntilUnixMilli})
		if err != nil {
			return result
		}
		transport := &http.Transport{DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(c, "unix", st.responseSocket)
		}, DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		request, err := http.NewRequestWithContext(ctx, "POST", "http://helper/block", bytes.NewReader(payload))
		if err != nil {
			return result
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			result.Status = "unacknowledged"
			result.Reason = "helper_outcome_unknown"
			return result
		}
		defer response.Body.Close()
		var reply struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&reply)
		if err != nil || response.StatusCode != http.StatusOK || (reply.Status != "applied" && reply.Status != "failed" && reply.Status != "rejected" && reply.Status != "unacknowledged") || len(reply.Reason) > 128 {
			result.Status = "unacknowledged"
			result.Reason = "invalid_helper_reply"
			return result
		}
		result.Status = reply.Status
		result.Reason = reply.Reason
		return result
	}
	return result
}
