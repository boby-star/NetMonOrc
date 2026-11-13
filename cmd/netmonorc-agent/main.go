package main

import (
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/boby-star/NetMonOrc/internal/agent"
	"github.com/boby-star/NetMonOrc/internal/secure"
)

func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
func run() error {
	endpoint := flag.String("controller", "127.0.0.1:9443", "Controller gRPC endpoint")
	serverName := flag.String("server-name", "localhost", "verified Controller TLS name")
	ca := flag.String("ca", "", "Controller CA bundle")
	cert := flag.String("cert", "", "Agent certificate")
	key := flag.String("key", "", "Agent private key file")
	dir := flag.String("data-dir", "", "exclusive private persistent Agent directory")
	allow := flag.String("allow-cidrs", "", "explicit target CIDR allowlist")
	blackbox := flag.String("blackbox-binary", "blackbox_exporter", "prepared exporter executable")
	flag.Parse()
	if *dir == "" || *allow == "" || *serverName == "" {
		return fmt.Errorf("data-dir, allow-cidrs and server-name are required")
	}
	var networks []netip.Prefix
	for _, raw := range strings.Split(*allow, ",") {
		n, e := netip.ParsePrefix(raw)
		if e != nil {
			return e
		}
		networks = append(networks, n)
	}
	tls, e := secure.Config(*ca, *cert, *key, false, *serverName)
	if e != nil {
		return e
	}
	leaf, e := x509.ParseCertificate(tls.Certificates[0].Certificate[0])
	if e != nil {
		return e
	}
	id, e := secure.Principal(leaf)
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log.Printf("Agent %s starting; identity derived from certificate", id)
	return agent.Run(ctx, *endpoint, tls, id, *dir, *blackbox, networks)
}
