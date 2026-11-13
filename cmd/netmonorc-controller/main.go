package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/coordinator"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/report"
	"github.com/boby-star/NetMonOrc/internal/secure"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"github.com/boby-star/NetMonOrc/internal/transport"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
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
	flag.Parse()
	b, e := os.ReadFile(*manifest)
	if e != nil {
		return e
	}
	p, e := config.Compile(b)
	if e != nil {
		return e
	}
	var profile diagnostics.Profile
	if p.Diagnostics != nil && p.Diagnostics.Enabled {
		if *profilePath == "" {
			return fmt.Errorf("enabled diagnostics require diagnostic-profile")
		}
		data, err := os.ReadFile(*profilePath)
		if err != nil {
			return err
		}
		profile, err = config.CompileDiagnostics(data)
		if err != nil {
			return err
		}
		p, err = config.ResolveDiagnostics(p, profile)
		if err != nil {
			return err
		}
	} else if *profilePath != "" {
		return fmt.Errorf("diagnostics disabled; profile cannot trigger a run")
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
	b, e = os.ReadFile(*database)
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dbctx, done := context.WithTimeout(ctx, 10*time.Second)
	store, e := storage.Open(dbctx, strings.TrimSpace(string(b)))
	done()
	if e != nil {
		return e
	}
	defer store.Close()
	lis, e := net.Listen("tcp", *listen)
	if e != nil {
		return e
	}
	defer lis.Close()
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tls)), grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(1<<20), grpc.MaxConcurrentStreams(8))
	agentServer := &transport.Server{Plan: p, Slots: slots, Store: store}
	pb.RegisterAgentServiceServer(server, agentServer)
	var diagnostic *coordinator.Coordinator
	if p.Diagnostics != nil && p.Diagnostics.Enabled {
		c, done := context.WithTimeout(ctx, 5*time.Second)
		e = store.LockController(c)
		done()
		if e != nil {
			return e
		}
		diagnostic, e = coordinator.New(p, profile, agentServer, store, *reportPath)
		if e != nil {
			return e
		}
		if e = diagnostic.RestoreReport(ctx); e != nil {
			return e
		}
		defer diagnostic.Wait()
	}

	errors := make(chan error, 2)
	go func() { errors <- server.Serve(lis) }()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			if e := assess(ctx, store, p, assessor, slots, *output, diagnostic, *webReport); e != nil {
				errors <- e
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
	stopped := make(chan struct{})
	go func() { server.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		server.Stop()
	}
	return result
}
func assess(ctx context.Context, store *storage.Store, p monitor.Plan, assessor *monitor.Assessor, slots []monitor.Slot, path string, diagnostic *coordinator.Coordinator, reportPath string) error {
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
	if reportPath != "" {
		d, err := store.LatestDiagnostic(c, p.Name)
		if err != nil {
			return err
		}
		html, err := report.Render(a, d)
		if err != nil {
			return err
		}
		return durable.AtomicWrite(reportPath, html)
	}
	return nil
}
