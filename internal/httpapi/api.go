// Package httpapi serves the implemented v1alpha1 browser and monitoring API.
package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/alertview"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"golang.org/x/sys/unix"
)

type Assessment struct {
	MonitorID     string                   `json:"monitorId"`
	State         string                   `json:"state"`
	CoverageState string                   `json:"coverageState"`
	ZoneStates    []monitor.ZoneAssessment `json:"zoneStates"`
	ReasonCodes   []string                 `json:"reasonCodes"`
	EvidenceIDs   []string                 `json:"evidenceIds"`
	EvaluatedAt   time.Time                `json:"evaluatedAt"`
	Revision      string                   `json:"revision"`
}
type Load func(context.Context, string) (monitor.Assessment, bool, error)
type Handler struct {
	alertAcknowledge AlertAcknowledgeCommand
	alerts           alertview.Reader
	alertEndpoint    string
	diagnosticStart  DiagnosticStartCommand
	diagnosticCancel DiagnosticCancelCommand
	diagnostics      diagnosticview.Reader
	agents           func() []agentstatus.View
	resources        *resources.Service
	expectedRevision string
	history          HistoryLoad
	auth             *auth.Service
	tokenEnabled     bool
	monitor          string
	tokenHash        [32]byte
	load             Load
	ui               fs.FS
	readSlots        chan struct{}
}

func LoadToken(file string) ([]byte, error) {
	fd, err := unix.Open(file, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("HTTP token file unavailable")
	}
	f := os.NewFile(uintptr(fd), "HTTP token")
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("HTTP token requires a private regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, 515))
	if err != nil || len(b) > 514 {
		return nil, fmt.Errorf("invalid HTTP token file")
	}
	b = []byte(strings.TrimSpace(string(b)))
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{32,512}$`).Match(b) {
		return nil, fmt.Errorf("HTTP token must have 32..512 URL-safe characters")
	}
	return b, nil
}
func New(name string, token []byte, load Load, ui fs.FS) (*Handler, error) {
	return NewWithSessions(name, token, load, ui, nil)
}
func NewWithSessions(name string, token []byte, load Load, ui fs.FS, sessions *auth.Service) (*Handler, error) {
	if name == "" || (len(token) < 32 && (sessions == nil || len(token) != 0)) || load == nil {
		return nil, fmt.Errorf("Monitor, strong token and assessment loader required")
	}
	if ui != nil {
		if info, err := fs.Stat(ui, "index.html"); err != nil || info.IsDir() {
			return nil, fmt.Errorf("built React index.html unavailable")
		}
	}
	return &Handler{auth: sessions, tokenEnabled: len(token) >= 32, monitor: name, tokenHash: sha256.Sum256(token), load: load, ui: ui, readSlots: make(chan struct{}, 1)}, nil
}
func NewServer(address string, h http.Handler) *http.Server {
	return &http.Server{Addr: address, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13}}
}
func failure(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Encoding these strings can fail only when the client disconnects.
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message, "requestId": rand.Text()}})
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		h.static(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/monitors/") && strings.HasSuffix(r.URL.Path, "/diagnostics") || (strings.HasPrefix(r.URL.Path, "/api/v1/diagnostics/") && strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == "POST") {
		h.diagnosticCommandRoutes(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/alerts/") && strings.HasSuffix(r.URL.Path, "/acknowledge") && r.Method == "POST" {
		h.acknowledgeAlert(w, r)
		return
	}
	if r.URL.Path == "/api/v1/alerts" || strings.HasPrefix(r.URL.Path, "/api/v1/alerts/") || r.URL.Path == "/api/v1/webhook/deliveries" {
		h.alertRoutes(w, r)
		return
	}
	if r.URL.Path == "/api/v1/diagnostics" || strings.HasPrefix(r.URL.Path, "/api/v1/diagnostics/") {
		h.diagnosticRoutes(w, r)
		return
	}
	if r.URL.Path == "/api/v1/agents" || strings.HasPrefix(r.URL.Path, "/api/v1/agents/") {
		h.agentRoutes(w, r)
		return
	}
	if r.URL.Path == "/api/v1/operations" || strings.HasPrefix(r.URL.Path, "/api/v1/operations/") {
		h.operationRoutes(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/resources/") || strings.HasPrefix(r.URL.Path, "/api/v1/config/") {
		h.resourceRoutes(w, r)
		return
	}
	if r.URL.Path == "/api/v1/users" || strings.HasPrefix(r.URL.Path, "/api/v1/users/") {
		h.userRoutes(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
		h.authRoutes(w, r)
		return
	}
	if r.URL.Path == "/api/v1/monitors/"+h.monitor+"/observations" {
		h.observations(w, r)
		return
	}
	if !h.authorize(w, r) {
		return
	}
	route := "/api/v1/monitors/" + h.monitor + "/assessment"
	if r.URL.Path != route {
		failure(w, 404, "NOT_FOUND", "Endpoint or Monitor not available")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "Read-only endpoint")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	select {
	case h.readSlots <- struct{}{}:
		defer func() { <-h.readSlots }()
	default:
		w.Header().Set("Retry-After", "1")
		failure(w, 429, "API_BUSY", "Assessment reader busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	a, found, err := h.load(ctx, h.monitor)
	age := time.Since(a.At)
	if err != nil || !found || a.Monitor != h.monitor || age > 5*time.Second || age < -5*time.Second || (h.expectedRevision != "" && a.Revision != h.expectedRevision) {
		failure(w, 503, "ASSESSMENT_UNAVAILABLE", "Fresh assessment temporarily unavailable")
		return
	}
	dto := Assessment{MonitorID: a.Monitor, State: a.State, CoverageState: "sufficient", ZoneStates: a.Zones, ReasonCodes: []string{}, EvidenceIDs: []string{}, EvaluatedAt: a.At, Revision: a.Revision}
	if dto.ZoneStates == nil {
		dto.ZoneStates = []monitor.ZoneAssessment{}
	}
	count := 0
	for _, z := range a.Zones {
		if z.Coverage != "complete" {
			dto.CoverageState = "degraded"
		}
		dto.ReasonCodes = append(dto.ReasonCodes, z.Reason)
		dto.EvidenceIDs = append(dto.EvidenceIDs, z.Evidence...)
		count += len(z.Evidence)
	}
	if count == 0 {
		dto.CoverageState = "unknown"
	}
	w.Header().Set("Content-Type", "application/json")
	// The persisted DTO is JSON-safe; a disconnected client needs no repair.
	_ = json.NewEncoder(w).Encode(dto)
}
func (h *Handler) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" || h.ui == nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	} else if !strings.HasPrefix(name, "assets/") || (path.Ext(name) != ".js" && path.Ext(name) != ".css") {
		http.NotFound(w, r)
		return
	}
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, h.ui, name)
}
