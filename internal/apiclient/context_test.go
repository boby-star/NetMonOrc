package apiclient

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

func TestPrivateContextAndTrustedIdentity(t *testing.T) {
	raw, err := auth.Token()
	if err != nil {
		t.Fatal(err)
	}
	secret := auth.APITokenPrefix + raw
	redirect := false
	unexpected := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { unexpected++; w.WriteHeader(500) }))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" || r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("incorrect identity request")
			w.WriteHeader(401)
			return
		}
		if redirect {
			http.Redirect(w, r, target.URL, 302)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"userId":"owner","username":"viewer","role":"viewer","expiresAt":null,"scopes":["resources:read"]}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(dir, "ca.pem")
	if err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	c := Context{Server: server.URL, Token: secret, CAFile: ca}
	if _, err = c.WhoAmI(context.Background()); err != nil {
		t.Fatal("trusted identity rejected", err)
	}
	untrusted := c
	untrusted.CAFile = ""
	if _, err = untrusted.WhoAmI(context.Background()); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	path := filepath.Join(dir, "private", "context.json")
	if err = Save(path, c); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe context permissions", err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Token != secret || loaded.Server != server.URL {
		t.Fatal("context not restored", err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); err == nil {
		t.Fatal("public context accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(path), "link.json")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(link); err == nil {
		t.Fatal("context symlink accepted")
	}
	redirect = true
	if _, err = c.WhoAmI(context.Background()); err == nil {
		t.Fatal("identity redirect accepted")
	}
	if unexpected != 0 {
		t.Fatal("bearer leaked through redirect")
	}
	invalid := c
	invalid.Server = strings.Replace(server.URL, "https:", "http:", 1)
	if err = invalid.Validate(); err == nil {
		t.Fatal("plaintext origin accepted")
	}
	if err = Logout(path); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("logout kept credential file")
	}
}
