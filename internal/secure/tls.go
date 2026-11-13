package secure

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"regexp"
)

var AgentID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func Config(caFile, certFile, keyFile string, server bool, serverName string) (*tls.Config, error) {
	b, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		return nil, fmt.Errorf("invalid CA bundle")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	c := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, RootCAs: pool, ServerName: serverName}
	if server {
		c.ClientAuth = tls.RequireAndVerifyClientCert
		c.ClientCAs = pool
	}
	return c, nil
}

// CN and remote IP are deliberately not authentication principals.
func Principal(cert *x509.Certificate) (string, error) {
	if len(cert.URIs) != 1 {
		return "", fmt.Errorf("exactly one Agent URI SAN required")
	}
	u := cert.URIs[0]
	if u.Scheme != "spiffe" || u.Host != "netmonorc" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", fmt.Errorf("invalid Agent URI SAN")
	}
	const prefix = "/agent/"
	if len(u.Path) <= len(prefix) || u.Path[:len(prefix)] != prefix || !AgentID.MatchString(u.Path[len(prefix):]) {
		return "", fmt.Errorf("invalid Agent identity")
	}
	return u.Path[len(prefix):], nil
}
