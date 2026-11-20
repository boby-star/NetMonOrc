package probe

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/boby-star/NetMonOrc/internal/component"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// ComponentReady checks only the local exporter endpoint, never /probe or the
// monitored target. Its bounded health response is not retained as evidence.
func ComponentReady(ctx context.Context, b monitor.ComponentBinding) bool {
	endpoint, err := component.NormalizeEndpoint(b.Endpoint)
	if err != nil || !b.Enabled || b.ModuleSet != component.ModuleSet {
		return false
	}
	child, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(child, http.MethodGet, endpoint+"/-/healthy", nil)
	if err != nil {
		return false
	}
	tr := &http.Transport{Proxy: nil, DialContext: dialLocalExporter, MaxResponseHeaderBytes: 4096, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4097))
	closeErr := resp.Body.Close()
	return resp.StatusCode == 200 && readErr == nil && closeErr == nil && len(data) <= 4096
}
