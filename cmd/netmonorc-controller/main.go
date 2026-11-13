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
	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
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
	flag.Parse()
	b, e := os.ReadFile(*manifest)
	if e != nil {
		return e
	}
	p, e := config.Compile(b)
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
	if _, e = monitor.Evaluate(p, slots, nil, time.Now()); e != nil {
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
	pb.RegisterAgentServiceServer(server, &transport.Server{Plan: p, Slots: slots, Store: store})
	errors := make(chan error, 2)
	go func() { errors <- server.Serve(lis) }()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			if e := assess(ctx, store, p, slots, *output); e != nil {
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
func assess(ctx context.Context, store *storage.Store, p monitor.Plan, slots []monitor.Slot, path string) error {
	c, done := context.WithTimeout(ctx, 5*time.Second)
	defer done()
	now := time.Now().UTC()
	observations, e := store.Latest(c, p, now)
	if e != nil {
		return e
	}
	a, e := monitor.Evaluate(p, slots, observations, now)
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
	return durable.AtomicWrite(path, b)
}
