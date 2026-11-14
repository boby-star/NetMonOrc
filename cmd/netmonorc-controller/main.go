package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/coordinator"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/httpapi"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/notify"
	"github.com/boby-star/NetMonOrc/internal/report"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/secure"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"github.com/boby-star/NetMonOrc/internal/transport"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}

var errReconcile = stderrors.New("reload desired Monitor")

func run() error {
	manifest := flag.String("config", "examples/http.yaml", "Monitor manifest")
	listen := flag.String("listen", "127.0.0.1:9443", "mTLS gRPC listen address")
	database := flag.String("database-file", "", "private file containing PostgreSQL DSN")
	ca := flag.String("ca", "", "Agent CA bundle")
	cert := flag.String("cert", "", "Controller certificate")
	key := flag.String("key", "", "Controller private key file")
	roster := flag.String("slots", "", "fixed planned agent=zone pairs")
	output := flag.String("assessment-file", ".local/assessment.json", "local assessment snapshot")
	profilePath := flag.String("diagnostic-profile", "", "referenced DiagnosticProfile file; no diagnostics without enabled binding")
	reportPath := flag.String("diagnostic-file", ".local/diagnostic.json", "latest persisted diagnostic report")
	webReport := flag.String("report-file", "", "optional private HTML report; no HTTP listener")
	staticComparison := flag.Bool("diagnostic-comparison-static", false, "research baseline: run every action from the same catalog; not a YAML mode")
	alertPath := flag.String("alert-rule", "", "optional single monitor-scope AlertRule file")
	endpointPath := flag.String("webhook-config", "", "single WebhookEndpoint file")
	webhookAllow := flag.String("webhook-allow-cidrs", "", "explicit HTTPS webhook destination CIDRs")
	webhookSecretDir := flag.String("webhook-secret-dir", "", "directory of preinstalled bearer secrets")
	webhookCA := flag.String("webhook-ca", "", "optional additional trusted webhook CA bundle")
	alertOutput := flag.String("alert-file", ".local/alert.json", "local alert state snapshot")
	deliveryOutput := flag.String("delivery-file", "", "optional private webhook delivery snapshot")
	httpListen := flag.String("http-listen", "", "optional HTTPS REST/UI listener")
	httpTokenFile := flag.String("http-token-file", "", "private read-only bootstrap bearer token file")
	uiDir := flag.String("http-ui-dir", "", "built React directory; optional")
	flag.Parse()
	parent, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		err := func() error {
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			if *httpListen == "" && (*httpTokenFile != "" || *uiDir != "") {
				return fmt.Errorf("http-token-file/http-ui-dir require http-listen")
			}
			b, e := os.ReadFile(*manifest)
			if e != nil {
				return e
			}
			bootstrap, e := resources.DecodeYAML(b)
			if e != nil {
				return e
			}
			execution := bootstrap
			execution.Metadata = resources.Metadata{Name: bootstrap.Metadata.Name}
			wire, e := json.Marshal(execution)
			if e != nil {
				return e
			}
			p, e := config.Compile(wire)
			if e != nil {
				return e
			}
			var profile diagnostics.Profile
			b, e = os.ReadFile(*database)
			if e != nil {
				return e
			}
			dbctx, done := context.WithTimeout(ctx, 10*time.Second)
			store, e := storage.Open(dbctx, strings.TrimSpace(string(b)))
			done()
			if e != nil {
				return e
			}
			defer store.Close()

			registry := resources.Registry{Active: p.Name, Zones: append([]string(nil), p.Zones...), StaticComparison: *staticComparison}
			if *profilePath != "" {
				data, err := os.ReadFile(*profilePath)
				if err != nil {
					return err
				}
				manifest, err := resources.DecodeYAML(data)
				if err != nil {
					return err
				}
				result, err := registry.CompileResource(manifest)
				if err != nil {
					return err
				}
				if _, err = store.SeedResource(ctx, manifest, result.Revision, registry); err != nil && !stderrors.Is(err, resources.ErrNotFound) {
					return err
				}
			}
			if _, err := store.SeedResource(ctx, bootstrap, "", registry); err != nil {
				return err
			}
			loaded, compiled, err := store.LoadCompiledMonitor(ctx, p.Name, registry)
			if err != nil {
				return err
			}
			registry = loaded
			p = *compiled.Monitor
			if p.Diagnostics != nil && p.Diagnostics.ProfileRef != "" {
				profile = registry.Profiles[p.Diagnostics.ProfileRef]
			}
			resourceService := resources.New(store, registry)
			var alertRule *alerting.Evaluator
			var endpoint webhook.Endpoint
			var sender *webhook.Sender
			if (*alertPath == "") != (*endpointPath == "") {
				return fmt.Errorf("alert-rule and webhook-config must be provided together")
			}
			if *alertPath != "" {
				data, err := os.ReadFile(*alertPath)
				if err != nil {
					return err
				}
				rule, err := config.CompileAlert(data)
				if err != nil {
					return err
				}
				data, err = os.ReadFile(*endpointPath)
				if err != nil {
					return err
				}
				endpoint, err = config.CompileWebhook(data)
				if err != nil {
					return err
				}
				if rule.Monitor != p.Name || rule.Channel != endpoint.Name {
					return fmt.Errorf("AlertRule monitor/channel reference mismatch")
				}
				alertRule, err = alerting.New(rule)
				if err != nil {
					return err
				}
				if endpoint.Enabled {
					var cidrs []netip.Prefix
					for _, text := range strings.Split(*webhookAllow, ",") {
						cidr, err := netip.ParsePrefix(text)
						if err != nil {
							return fmt.Errorf("explicit webhook-allow-cidrs required")
						}
						cidrs = append(cidrs, cidr)
					}
					sender, err = webhook.New(endpoint, cidrs, *webhookSecretDir, *webhookCA)
					if err != nil {
						return err
					}
					defer sender.Close()
				}
			}
			assessor, e := monitor.NewAssessor(p)
			if e != nil {
				return e
			}
			var slots []monitor.Slot
			for _, entry := range strings.Split(*roster, ",") {
				id, z, ok := strings.Cut(entry, "=")
				if !ok || !secure.AgentID.MatchString(id) {
					return fmt.Errorf("invalid fixed Agent roster")
				}
				slots = append(slots, monitor.Slot{Agent: id, Zone: z})
			}
			if len(slots) != len(p.Zones) {
				return fmt.Errorf("one fixed Agent is required per configured zone")
			}
			if _, e = assessor.Evaluate(slots, nil, time.Now()); e != nil {
				return e
			}
			tls, e := secure.Config(*ca, *cert, *key, true, "")
			if e != nil {
				return e
			}
			lis, e := net.Listen("tcp", *listen)
			if e != nil {
				return e
			}
			defer lis.Close()
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tls)), grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(1<<20), grpc.MaxConcurrentStreams(8))
			agentServer := &transport.Server{Plan: p, Slots: slots, Store: store}
			pb.RegisterAgentServiceServer(server, agentServer)
			{
				c, done := context.WithTimeout(ctx, 5*time.Second)
				e = store.LockController(c)
				done()
				if e != nil {
					return e
				}
			}
			var notifications *notify.Service
			if alertRule != nil {
				notifications = &notify.Service{Store: store, Rule: alertRule, Endpoint: endpoint, Sender: sender}
			}
			var diagnostic *coordinator.Coordinator
			if p.Diagnostics != nil && p.Diagnostics.Enabled {
				diagnostic, e = coordinator.New(p, profile, agentServer, store, *reportPath)
				if e != nil {
					return e
				}
				if e = diagnostic.RestoreReport(ctx); e != nil {
					return e
				}
				defer diagnostic.Wait()
			}

			var httpServer *http.Server
			if *httpListen != "" {
				var token []byte
				if *httpTokenFile != "" {
					var err error
					token, err = httpapi.LoadToken(*httpTokenFile)
					if err != nil {
						return err
					}
				}
				var assets fs.FS
				if *uiDir != "" {
					assets = os.DirFS(*uiDir)
				}
				handler, err := httpapi.NewWithSessions(p.Name, token, func(c context.Context, name string) (monitor.Assessment, bool, error) {
					return store.ReadLiveAssessment(c, name, p.Revision)
				}, assets, auth.New(store))
				if err != nil {
					return err
				}
				handler.SetHistoryLoader(store.ReadObservations)
				handler.SetDiagnostics(store)
				loadSession := func(c context.Context, id string) (diagnosticview.Session, error) {
					v, found, e := store.DiagnosticSession(c, id)
					if e != nil {
						return v, e
					}
					if !found {
						return v, resources.ErrNotFound
					}
					return v, nil
				}
				handler.SetDiagnosticCommands(func(c context.Context, actor auth.AdminCredential, name string, request diagnosticview.Start) (diagnosticview.Session, error) {
					var empty diagnosticview.Session
					if name != p.Name {
						return empty, resources.ErrNotFound
					}
					request.Monitor = name
					request, e := coordinator.NormalizeStart(request)
					if e != nil {
						return empty, e
					}
					id, e := store.RecoverDiagnosticStart(c, actor, request)
					if e != nil {
						return empty, e
					}
					if id != "" {
						return loadSession(c, id)
					}
					if diagnostic == nil {
						return empty, resources.ErrConflict
					}
					id, e = diagnostic.StartManual(ctx, c, actor, request)
					if e != nil {
						return empty, e
					}
					return loadSession(c, id)
				}, func(c context.Context, actor auth.AdminCredential, id string) (diagnosticview.Session, error) {
					if e := store.CancelDiagnostic(c, actor, id); e != nil {
						return diagnosticview.Session{}, e
					}
					if diagnostic != nil {
						diagnostic.CancelRun(id)
					}
					return loadSession(c, id)
				})
				handler.SetResources(resourceService, p.Revision)
				handler.SetAgents(func() []agentstatus.View { return agentServer.AgentStatuses(time.Now()) })
				httpServer = httpapi.NewServer(*httpListen, handler)
			}
			errors := make(chan error, 4)
			notificationDone := make(chan struct{})
			if notifications != nil {
				go func() {
					defer close(notificationDone)
					if err := notifications.Run(ctx); err != nil {
						select {
						case errors <- err:
						case <-ctx.Done():
						}
					}
				}()
			} else {
				close(notificationDone)
			}
			if httpServer != nil {
				log.Print("HTTPS API enabled; browser sessions and optional bootstrap reader")
				go func() {
					err := httpServer.ListenAndServeTLS(*cert, *key)
					if err != nil && !stderrors.Is(err, http.ErrServerClosed) {
						errors <- err
					}
				}()
			}
			go func() { errors <- server.Serve(lis) }()
			assessmentDone := make(chan struct{})
			go func() {
				defer close(assessmentDone)
				tick := time.NewTicker(time.Second)
				defer tick.Stop()
				for {
					if e := assess(ctx, store, p, assessor, slots, *output, diagnostic, *webReport, notifications, *alertOutput, *deliveryOutput); e != nil {
						errors <- e
						return
					}
					opctx, opdone := context.WithTimeout(ctx, 3*time.Second)
					opErr := store.ReconcileOperations(opctx, p.Name, p.Revision, slots, agentServer.AppliedAssignments(time.Now()))
					opdone()
					if opErr != nil && ctx.Err() == nil {
						log.Print("Operation reconciliation deferred; regular assessment continues")
					}
					latest, found, err := store.Resource(ctx, p.Name)
					if err != nil {
						if ctx.Err() == nil {
							errors <- err
						}
						return
					}
					if !found || latest.Revision != p.Revision {
						errors <- errReconcile
						return
					}
					select {
					case <-ctx.Done():
						return
					case <-tick.C:
					}
				}
			}()
			log.Print("Controller listening with mTLS; fixed roster loaded")
			var result error
			select {
			case <-ctx.Done():
			case result = <-errors:
			}
			cancel()
			if httpServer != nil {
				shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
				_ = httpServer.Shutdown(shutdown)
				done()
			}
			<-assessmentDone
			<-notificationDone
			stopped := make(chan struct{})
			go func() { server.GracefulStop(); close(stopped) }()
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				server.Stop()
			}
			return result
		}()
		if parent.Err() != nil {
			return nil
		}
		if !stderrors.Is(err, errReconcile) {
			return err
		}
		log.Print("Reloading persisted Monitor; previous sessions and assessment revision fenced")
	}
}
func assess(ctx context.Context, store *storage.Store, p monitor.Plan, assessor *monitor.Assessor, slots []monitor.Slot, path string, diagnostic *coordinator.Coordinator, reportPath string, notifications *notify.Service, alertPath, deliveryPath string) error {
	var alertState *alerting.State
	var delivery *webhook.Summary
	c, done := context.WithTimeout(ctx, 5*time.Second)
	defer done()
	now := time.Now().UTC()
	observations, e := store.Latest(c, p, now)
	if e != nil {
		return e
	}
	a, e := assessor.Evaluate(slots, observations, now)
	if e != nil {
		return e
	}
	if e = store.SaveAssessment(c, a); e != nil {
		return e
	}
	b, e := json.MarshalIndent(a, "", "  ")
	if e != nil {
		return e
	}
	if e = durable.AtomicWrite(path, b); e != nil {
		return e
	}
	if diagnostic != nil {
		if e = store.ControllerAlive(c); e != nil {
			return e
		}
		if e = diagnostic.Tick(ctx, a, observations); e != nil {
			return e
		}
	}
	if notifications != nil {
		if diagnostic == nil {
			if e = store.ControllerAlive(c); e != nil {
				return e
			}
		}
		state, err := notifications.Evaluate(c, a)
		if err != nil {
			return err
		}
		alertState = &state
		summary, err := store.WebhookSummary(c, notifications.Endpoint, notifications.Rule.Rule.Name, p.Name)
		if err != nil {
			return err
		}
		delivery = &summary
		if deliveryPath != "" {
			data, err := json.MarshalIndent(summary, "", "  ")
			if err != nil {
				return err
			}
			if err = durable.AtomicWrite(deliveryPath, data); err != nil {
				return err
			}
		}
		b, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		if err = durable.AtomicWrite(alertPath, b); err != nil {
			return err
		}
	}
	if reportPath != "" {
		d, err := store.LatestDiagnostic(c, p.Name)
		if err != nil {
			return err
		}
		html, err := report.RenderWithDelivery(a, d, alertState, delivery)
		if err != nil {
			return err
		}
		return durable.AtomicWrite(reportPath, html)
	}
	return nil
}
