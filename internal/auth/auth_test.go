package auth

import (
	"strings"
	"testing"
)

func TestArgon2AndSessionSecrets(t *testing.T) {
	password := "long-random-test-password"
	a, e := HashPassword(password)
	if e != nil {
		t.Fatal(e)
	}
	b, e := HashPassword(password)
	if e != nil {
		t.Fatal(e)
	}
	if a == b || strings.Contains(a, password) {
		t.Fatal("salt or password storage failure")
	}
	if ok, e := VerifyPassword(password, a); e != nil || !ok {
		t.Fatal("password rejected", e)
	}
	if ok, e := VerifyPassword("wrong", a); e != nil || ok {
		t.Fatal("wrong password accepted", e)
	}
	if _, e := VerifyPassword(password, strings.Replace(a, "m=65536", "m=999999999", 1)); e == nil {
		t.Fatal("unbounded hash parameters accepted")
	}
	raw, e := Token()
	if e != nil || len(raw) != 43 {
		t.Fatal("invalid session entropy", e)
	}
	if string(HashToken(raw)) == raw || CSRF(raw) == raw || CSRF(raw) != CSRF(raw) {
		t.Fatal("session/CSRF secrets mixed")
	}
	for _, scope := range Scopes("viewer") {
		if strings.HasSuffix(scope, ":write") || strings.HasSuffix(scope, ":admin") {
			t.Fatal("viewer write permission")
		}
	}
}
