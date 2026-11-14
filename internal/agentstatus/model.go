// Package agentstatus defines the read model for fixed-roster Agent sessions.
package agentstatus

import "time"

type View struct {
	AgentID             string            `json:"agentId"`
	Name                string            `json:"name"`
	OnlineState         string            `json:"onlineState"`
	NetworkZone         string            `json:"networkZone"`
	Labels              map[string]string `json:"labels"`
	Capabilities        []string          `json:"capabilities,omitempty"`
	AppliedRevision     *string           `json:"appliedRevision"`
	LastHeartbeat       *time.Time        `json:"lastHeartbeat"`
	CertExpiresAt       *time.Time        `json:"certExpiresAt"`
	MaxConcurrentProbes *int              `json:"maxConcurrentProbes"`
	ActiveProbes        *int              `json:"activeProbes"`
}
type Page struct {
	Items      []View  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
