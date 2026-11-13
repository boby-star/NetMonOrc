package secure

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net/url"
	"testing"
)

func TestPrincipalRequiresExactURI(t *testing.T) {
	for _, tc := range []struct {
		uri   string
		valid bool
	}{{"spiffe://netmonorc/agent/branch", true}, {"spiffe://other/agent/branch", false}, {"spiffe://netmonorc/agent/branch/extra", false}, {"spiffe://netmonorc/agent/branch?role=admin", false}, {"spiffe://netmonorc/agent/%62ranch", false}} {
		u, e := url.Parse(tc.uri)
		if e != nil {
			t.Fatal(e)
		}
		_, e = Principal(&x509.Certificate{URIs: []*url.URL{u}})
		if (e == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.uri, e)
		}
	}
	if _, e := Principal(&x509.Certificate{Subject: pkix.Name{CommonName: "branch"}}); e == nil {
		t.Fatal("CN accepted without URI SAN")
	}
}
