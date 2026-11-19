package config

import (
	"os"
	"strings"
	"testing"
)

func TestHTTPPolicyConfigurationAndHeaders(t *testing.T) {
	source, _ := os.ReadFile("../../examples/http.yaml")
	with := func(fields string) []byte {
		return []byte(strings.Replace(string(source), "expectedStatus: [200]", "expectedStatus: [200]\n        "+fields, 1))
	}
	for _, bad := range []string{"headers: {Authorization: secret}", "headers: {Cookie: secret}", "headers: {X-Api-Key: secret}", "headers: {Host: other}", "headers: {X-Probe: true}", "headers: {X-Probe: one, x-probe: two}", "connectIP: hostname.invalid", "verifyTLS: false"} {
		if _, err := Compile(with(bad)); err == nil {
			t.Fatal("invalid HTTP policy admitted", bad)
		}
	}
	a, err := Compile(with("headers: {X-Probe: safe}"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(with("headers: {x-probe: safe}"))
	if err != nil || a.Revision != b.Revision {
		t.Fatal("header case changed semantic revision", err)
	}
	data := string(with("connectIP: 127.0.0.1"))
	data = strings.Replace(data, "runner: native-http", "runner: auto", 1)
	data = strings.Replace(data, "http://127.0.0.1:18080/", "https://example.com/", 1)
	p, err := Compile([]byte(data))
	if err != nil || p.Checks[0].Runner != "native-http" || !p.Checks[0].NeedsHTTPPolicy() {
		t.Fatal("connectIP auto runner not statically selected", err)
	}
	if _, err = Compile([]byte(strings.Replace(data, "runner: auto", "runner: blackbox", 1))); err == nil {
		t.Fatal("connectIP silently ignored by blackbox")
	}
}
