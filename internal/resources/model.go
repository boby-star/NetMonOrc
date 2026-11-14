// Package resources manages validated declarative resources and their versions.
package resources

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

var ErrInvalid = errors.New("invalid or unsupported resource")
var ErrConflict = errors.New("resource exists or has an active reference")
var ErrNotFound = errors.New("resource not found")
var ErrVersion = errors.New("resource version changed")

type Metadata struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}
type Manifest struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   Metadata        `json:"metadata"`
	Spec       json.RawMessage `json:"spec"`
}
type View struct {
	Manifest        Manifest  `json:"manifest"`
	UID             string    `json:"uid"`
	Generation      int64     `json:"generation"`
	ResourceVersion string    `json:"resourceVersion"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Status          *Status   `json:"status,omitempty"`
	Revision        string    `json:"-"`
}
type Status struct {
	State           string      `json:"state"`
	DesiredRevision string      `json:"desiredRevision"`
	Conditions      []Condition `json:"conditions"`
}
type Condition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type Page struct {
	Items      []View  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
type Revision struct {
	UID             string    `json:"uid"`
	Generation      int64     `json:"generation"`
	ResourceVersion string    `json:"resourceVersion"`
	Manifest        Manifest  `json:"manifest"`
	ChangedAt       time.Time `json:"changedAt"`
	Actor           string    `json:"actor"`
}
type Repository interface {
	Catalog(context.Context) ([]Manifest, error)
	ResourceKind(context.Context, string, string) (View, bool, error)
	ResourcesKind(context.Context, string, string, string, int) ([]View, error)
	ResourceHistoryKind(context.Context, string, string, string, int) ([]Revision, error)
	DeleteResourceKind(context.Context, auth.AdminCredential, string, string, string, Registry) error

	Resource(context.Context, string) (View, bool, error)
	Resources(context.Context, string, string, int) ([]View, error)
	SaveResource(context.Context, auth.AdminCredential, Manifest, Registry, *string) (View, error)
	DeleteResource(context.Context, auth.AdminCredential, string, string) error
	ResourceHistory(context.Context, string, string, int) ([]Revision, error)
	RecoverApply(context.Context, auth.AdminCredential, string, []byte) (Operation, bool, error)
	ApplyResources(context.Context, auth.AdminCredential, Batch, Registry, []byte) (Operation, error)
	Operation(context.Context, string) (Operation, bool, error)
	Operations(context.Context, string, string, int) ([]Operation, error)
}
