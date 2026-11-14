package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

func testUserManagement(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, username, password string) {
	t.Helper()
	if _, e := s.pool.Exec(ctx, `UPDATE nmo_users SET role='admin',disabled=false,failed_logins=0,locked_until=NULL,created_at=NULL WHERE username=$1`, username); e != nil {
		t.Fatal(e)
	}
	principal, session, e := auth.New(s).Login(ctx, username, password)
	if e != nil {
		t.Fatal(e)
	}
	client := server.Client()
	request := func(method, path string, body any, tag, csrf, bearer string) (int, []byte, string) {
		t.Helper()
		var input io.Reader
		if body != nil {
			b, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			input = bytes.NewReader(b)
		}
		r, e := http.NewRequest(method, server.URL+path, input)
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Origin", server.URL)
		r.Header.Set("Cookie", "netmonorc_session="+session)
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if tag != "" {
			r.Header.Set("If-Match", tag)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, data, response.Header.Get("ETag")
	}
	code, data, ownTag := request("GET", "/api/v1/users/"+principal.UserID, nil, "", "", "")
	var own auth.UserView
	if code != 200 || ownTag == "" || json.Unmarshal(data, &own) != nil || own.ID != principal.UserID || strings.Contains(string(data), "password") || strings.Contains(string(data), "createdAt") {
		t.Fatal("admin user read", code)
	}
	for _, patch := range []any{map[string]bool{"disabled": true}, map[string]string{"role": "viewer"}} {
		if code, _, _ := request("PATCH", "/api/v1/users/"+own.ID, patch, ownTag, principal.CSRFToken, ""); code != 409 {
			t.Fatal("last administrator lost", code)
		}
	}
	viewerName := "viewer-" + newID()
	viewerPassword := "viewer-initial-password-long"
	input := map[string]string{"username": viewerName, "role": "viewer", "initialPassword": viewerPassword}
	if code, _, _ := request("POST", "/api/v1/users", input, "", "", ""); code != 403 {
		t.Fatal("create without CSRF", code)
	}
	code, data, tag := request("POST", "/api/v1/users", input, "", principal.CSRFToken, "")
	var viewer auth.UserView
	if code != 201 || tag == "" || json.Unmarshal(data, &viewer) != nil || viewer.Role != "viewer" || (viewer.CreatedAt == nil || viewer.CreatedAt.IsZero()) || strings.Contains(string(data), viewerPassword) || strings.Contains(string(data), "hash") {
		t.Fatal("create user", code)
	}
	if code, _, _ := request("POST", "/api/v1/users", input, "", principal.CSRFToken, ""); code != 409 {
		t.Fatal("duplicate user", code)
	}
	viewerPrincipal, viewerSession, e := auth.New(s).Login(ctx, viewerName, viewerPassword)
	if e != nil {
		t.Fatal("new user cannot login", e)
	}
	reader, e := auth.New(s).IssueToken(ctx, viewerSession, auth.APITokenCreate{Name: "viewer-reader"})
	if e != nil {
		t.Fatal(e)
	}
	if code, _, _ := request("GET", "/api/v1/users", nil, "", "", reader.Token); code != 403 {
		t.Fatal("viewer enumerated users", code)
	}
	if code, _, _ := request("POST", "/api/v1/users", input, "", viewerPrincipal.CSRFToken, reader.Token); code != 403 {
		t.Fatal("viewer created identity", code)
	}
	limited, e := auth.New(s).IssueToken(ctx, session, auth.APITokenCreate{Name: "limited-admin", Scopes: []string{"resources:read"}})
	if e != nil {
		t.Fatal(e)
	}
	if code, _, _ := request("GET", "/api/v1/users", nil, "", "", limited.Token); code != 403 {
		t.Fatal("admin token bypassed scope", code)
	}
	patch := map[string]bool{"disabled": true}
	if code, _, _ := request("PATCH", "/api/v1/users/"+viewer.ID, patch, "", principal.CSRFToken, ""); code != 428 {
		t.Fatal("missing If-Match", code)
	}
	if code, _, _ := request("PATCH", "/api/v1/users/"+viewer.ID, patch, `"`, principal.CSRFToken, ""); code != 400 {
		t.Fatal("malformed If-Match", code)
	}
	if code, _, _ := request("PATCH", "/api/v1/users/"+viewer.ID, patch, `"user-`+viewer.ID+`-0"`, principal.CSRFToken, ""); code != 412 {
		t.Fatal("stale If-Match", code)
	}
	if code, _, _ := request("PATCH", "/api/v1/users/"+viewer.ID, map[string]any{"disabled": nil}, tag, principal.CSRFToken, ""); code != 422 {
		t.Fatal("null disabled accepted", code)
	}
	code, data, disabledTag := request("PATCH", "/api/v1/users/"+viewer.ID, patch, tag, principal.CSRFToken, "")
	if code != 200 || disabledTag == tag || json.Unmarshal(data, &viewer) != nil || !viewer.Disabled {
		t.Fatal("disable user", code)
	}
	if _, e = auth.New(s).Me(ctx, viewerSession); e != auth.ErrCredentials {
		t.Fatal("disabled session survived", e)
	}
	if _, e = auth.New(s).TokenPrincipal(ctx, reader.Token); e != auth.ErrCredentials {
		t.Fatal("disabled token survived", e)
	}
	if code, _, _ := request("PATCH", "/api/v1/users/"+viewer.ID, patch, tag, principal.CSRFToken, ""); code != 412 {
		t.Fatal("lost update accepted", code)
	}
	code, _, enabledTag := request("PATCH", "/api/v1/users/"+viewer.ID, map[string]bool{"disabled": false}, disabledTag, principal.CSRFToken, "")
	if code != 200 {
		t.Fatal("reenable user", code)
	}
	if _, e = auth.New(s).Me(ctx, viewerSession); e != auth.ErrCredentials {
		t.Fatal("reenable resurrected session", e)
	}
	if _, e = auth.New(s).TokenPrincipal(ctx, reader.Token); e != auth.ErrCredentials {
		t.Fatal("reenable resurrected PAT", e)
	}
	_, freshViewer, e := auth.New(s).Login(ctx, viewerName, viewerPassword)
	if e != nil {
		t.Fatal("reenabled identity cannot login", e)
	}
	freshToken, e := auth.New(s).IssueToken(ctx, freshViewer, auth.APITokenCreate{Name: "fresh-viewer"})
	if e != nil {
		t.Fatal(e)
	}
	code, _, promotedTag := request("PATCH", "/api/v1/users/"+viewer.ID, map[string]string{"role": "admin"}, enabledTag, principal.CSRFToken, "")
	if code != 200 {
		t.Fatal("role change", code)
	}
	if _, e = auth.New(s).Me(ctx, freshViewer); e != auth.ErrCredentials {
		t.Fatal("role change retained session", e)
	}
	if _, e = auth.New(s).TokenPrincipal(ctx, freshToken.Token); e != auth.ErrCredentials {
		t.Fatal("role change retained PAT", e)
	}
	if code, _, _ := request("PATCH", "/api/v1/users/"+viewer.ID, map[string]string{"role": "viewer"}, promotedTag, principal.CSRFToken, ""); code != 200 {
		t.Fatal("role demotion", code)
	}
	management, e := auth.New(s).IssueToken(ctx, session, auth.APITokenCreate{Name: "admin-management", Scopes: []string{"system:admin"}})
	if e != nil {
		t.Fatal(e)
	}
	adminName := "admin-" + newID()
	adminPassword := "second-admin-password-long"
	code, data, _ = request("POST", "/api/v1/users", map[string]string{"username": adminName, "role": "admin", "initialPassword": adminPassword}, "", "", management.Token)
	var second auth.UserView
	if code != 201 || json.Unmarshal(data, &second) != nil {
		t.Fatal("scoped PAT cannot create user", code)
	}
	code, data, _ = request("GET", "/api/v1/users?limit=1", nil, "", "", management.Token)
	var page auth.UserPage
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.NextCursor == nil || strings.Contains(string(data), "password") {
		t.Fatal("user metadata pagination", code)
	}
	first := page.Items[0].ID
	code, data, _ = request("GET", "/api/v1/users?limit=1&cursor="+*page.NextCursor, nil, "", "", management.Token)
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ID == first {
		t.Fatal("user page repeated", code)
	}
	// Simultaneous last-admin mutations use separate transactions, as separate
	// processes would. Exactly one may succeed; HTTP admission is not the invariant.
	secondPrincipal, secondSession, e := auth.New(s).Login(ctx, adminName, adminPassword)
	if e != nil {
		t.Fatal(e)
	}
	disable := true
	start := make(chan struct{})
	results := make(chan error, 2)
	actors := []struct {
		credential auth.AdminCredential
		target     string
		version    int64
	}{
		{auth.AdminCredential{UserID: own.ID, Hash: auth.HashToken(session)}, own.ID, 1},
		{auth.AdminCredential{UserID: secondPrincipal.UserID, Hash: auth.HashToken(secondSession)}, second.ID, 1},
	}
	for _, entry := range actors {
		go func() {
			<-start
			_, err := s.PatchUser(ctx, entry.credential, entry.target, entry.version, auth.UserPatch{Disabled: &disable})
			results <- err
		}()
	}
	close(start)
	success, conflict := 0, 0
	for range actors {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, auth.ErrConflict) {
			conflict++
		} else {
			t.Fatal("concurrent admin transition", err)
		}
	}
	var count int
	if e = s.pool.QueryRow(ctx, `SELECT count(*) FROM nmo_users WHERE role='admin' AND NOT disabled`).Scan(&count); e != nil {
		t.Fatal(e)
	}
	if success != 1 || conflict != 1 || count != 1 {
		t.Fatal("last-admin invariant under concurrency", success, conflict, count)
	}
}
