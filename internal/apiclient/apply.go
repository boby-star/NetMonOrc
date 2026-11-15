package apiclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/boby-star/NetMonOrc/internal/resources"
)

// Apply retries only the config.apply contract, whose stable per-user requestId
// makes an interrupted response recoverable without committing a second operation.
func (c *Client) Apply(ctx context.Context, batch resources.Batch) ([]byte, error) {
	if batch.RequestID == "" {
		return nil, fmt.Errorf("apply requires a stable requestId")
	}
	for {
		response, err := c.JSON(ctx, "POST", "/api/v1/config/apply", batch)
		if err == nil {
			return response, nil
		}
		var transport *RequestError
		var api *APIError
		transient := (errors.As(err, &transport) && transport.Transient) || (errors.As(err, &api) && (api.Status == 429 || api.Status == 503))
		if !transient || ctx.Err() != nil {
			return nil, err
		}
		timer := time.NewTimer(300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
