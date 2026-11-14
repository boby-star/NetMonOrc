package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API request rejected (HTTP %d %s)", e.Status, e.Code)
}

type Client struct {
	server, token string
	http          *http.Client
	transport     *http.Transport
}

func (c Context) Client() (*Client, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("trusted CA bundle unavailable")
		}
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}, MaxIdleConns: 1, MaxConnsPerHost: 1, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 16384}
	return &Client{server: strings.TrimSuffix(c.Server, "/"), token: c.Token, transport: transport, http: &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Close() { c.transport.CloseIdleConnections() }

var errorCode = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)

// JSON preserves response integers by returning validated bytes rather than a
// float64-backed map. Error bodies are never printed, as they may echo secrets.
func (c *Client) JSON(ctx context.Context, method, path string, body any) ([]byte, error) {
	u, err := url.Parse(path)
	if err != nil || u.Host != "" || u.Scheme != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/api/v1/") {
		return nil, fmt.Errorf("API-relative path required")
	}
	var data []byte
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil || len(data) > 1<<20 {
			return nil, fmt.Errorf("invalid request or body exceeds 1 MiB")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.server+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid API request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTPS request failed; verify TLS trust, connectivity and deadline")
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil || len(b) > 16<<20 {
		return nil, fmt.Errorf("response too large or incomplete; reduce page size")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(b, &envelope)
		code := envelope.Error.Code
		if !errorCode.MatchString(code) {
			code = "REQUEST_FAILED"
		}
		return nil, &APIError{response.StatusCode, code}
	}
	if !json.Valid(b) {
		return nil, fmt.Errorf("API returned invalid JSON")
	}
	return b, nil
}
