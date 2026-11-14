package coordinator

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

// NormalizeStart fixes defaults before computing the durable request identity.
func NormalizeStart(request diagnosticview.Start) (diagnosticview.Start, error) {
	if request.Reason == "" {
		request.Reason = "manual"
	}
	if request.ProfileRef == "" || len(request.ProfileRef) > 63 || (request.Reason != "manual" && request.Reason != "trigger") || len(request.RequestID) > 128 || !utf8.ValidString(request.RequestID) || strings.ContainsFunc(request.RequestID, unicode.IsControl) {
		return request, resources.ErrInvalid
	}
	return request, nil
}
func (c *Coordinator) StartManual(runtime, requestContext context.Context, actor auth.AdminCredential, request diagnosticview.Start) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if prior, err := c.Store.RecoverDiagnosticStart(requestContext, actor, request); err != nil {
		return "", err
	} else if prior != "" {
		return prior, nil
	}
	if runtime.Err() != nil || c.running || time.Since(c.assessment.At) > 5*time.Second || time.Until(c.assessment.At) > 5*time.Second {
		return "", resources.ErrConflict
	}
	agents := c.Server.Executors(time.Now())
	if len(agents) == 0 {
		return "", resources.ErrConflict
	}
	resource, found, err := c.Store.ResourceKind(requestContext, "DiagnosticProfile", request.ProfileRef)
	if err != nil {
		return "", err
	}
	if !found {
		return "", resources.ErrNotFound
	}
	compiled, err := (resources.Registry{}).CompileResource(resource.Manifest)
	if err != nil {
		return "", resources.ErrInvalid
	}
	plan := c.Plan
	binding := *plan.Diagnostics
	binding.ProfileRef = request.ProfileRef
	binding.ProfileHash = compiled.Revision
	plan.Diagnostics = &binding
	started := time.Now()
	id, err := c.Store.StartManualDiagnostic(requestContext, plan, *compiled.Profile, actor, request)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", resources.ErrConflict
	}
	// A concurrent idempotency replay is recovered before reaching this method
	// by the HTTP command adapter; admission itself is also transactionally guarded.
	c.launch(runtime, id, plan, *compiled.Profile, agents, c.assessment, c.observations, started)
	return id, nil
}
