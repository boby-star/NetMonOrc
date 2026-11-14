package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

func testAPITokenUseCases(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	request := func(method, path string, body any, bearer, csrf string) (int, []byte) {
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
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
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
		return response.StatusCode, data
	}
	in := map[string]any{"name": "cli-reader", "scopes": []string{"resources:read"}}
	if code, _ := request("POST", "/api/v1/auth/tokens", in, "", ""); code != 403 {
		t.Fatal("token issuance missing CSRF", code)
	}
	code, data := request("POST", "/api/v1/auth/tokens", in, "", session.CSRFToken)
	var issued auth.APITokenIssued
	if code != 201 || json.Unmarshal(data, &issued) != nil || !strings.HasPrefix(issued.Token, auth.APITokenPrefix) || issued.ExpiresAt != nil {
		t.Fatal("token issuance", code)
	}
	var row string
	if e := s.pool.QueryRow(ctx, `SELECT to_jsonb(t)::text FROM nmo_api_tokens t WHERE id=$1`, issued.ID).Scan(&row); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(row, issued.Token) {
		t.Fatal("plaintext API token stored")
	}
	if _, e := auth.New(s).TokenPrincipal(ctx, issued.Token); e != nil {
		t.Fatal("API token not durable", e)
	}
	code, data = request("GET", "/api/v1/auth/me", nil, issued.Token, "")
	var principal auth.SessionView
	if code != 200 || json.Unmarshal(data, &principal) != nil || principal.UserID != session.UserID || principal.ExpiresAt != nil || principal.CSRFToken != "" {
		t.Fatal("PAT identity", code)
	}
	if code, _ = request("GET", "/api/v1/monitors/service/assessment", nil, issued.Token, ""); code != 200 {
		t.Fatal("PAT cannot read assessment", code)
	}
	if code, _ = request("GET", "/api/v1/monitors/service/observations", nil, issued.Token, ""); code != 200 {
		t.Fatal("PAT cannot read history", code)
	}
	if code, _ = request("GET", "/api/v1/auth/me", nil, "invalid-token", ""); code != 401 {
		t.Fatal("invalid bearer fell back to cookie", code)
	}
	if code, _ = request("POST", "/api/v1/auth/tokens", in, issued.Token, session.CSRFToken); code != 403 {
		t.Fatal("PAT minted another credential", code)
	}
	code, data = request("POST", "/api/v1/auth/tokens", map[string]any{"name": "write-only", "scopes": []string{"resources:write"}}, "", session.CSRFToken)
	var writeOnly auth.APITokenIssued
	if code != 201 || json.Unmarshal(data, &writeOnly) != nil {
		t.Fatal("scoped token creation", code)
	}
	if code, _ = request("GET", "/api/v1/monitors/service/assessment", nil, writeOnly.Token, ""); code != 403 {
		t.Fatal("missing read scope accepted", code)
	}
	if _, e := s.pool.Exec(ctx, `UPDATE nmo_users SET role='viewer' WHERE id=$1`, session.UserID); e != nil {
		t.Fatal(e)
	}
	code, data = request("GET", "/api/v1/auth/me", nil, writeOnly.Token, "")
	if code != 200 || json.Unmarshal(data, &principal) != nil || principal.Role != "viewer" || len(principal.Scopes) != 0 {
		t.Fatal("PAT retained downgraded privileges", code)
	}
	if code, _ = request("POST", "/api/v1/auth/tokens", map[string]any{"name": "escalation", "scopes": []string{"system:admin"}}, "", session.CSRFToken); code != 403 {
		t.Fatal("viewer granted admin", code)
	}
	if _, e := s.pool.Exec(ctx, `UPDATE nmo_users SET role='admin' WHERE id=$1`, session.UserID); e != nil {
		t.Fatal(e)
	}
	code, data = request("GET", "/api/v1/auth/tokens?limit=1", nil, "", "")
	var page auth.APITokenPage
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.NextCursor == nil || strings.Contains(string(data), issued.Token) {
		t.Fatal("token metadata page", code)
	}
	first := page.Items[0].ID
	code, data = request("GET", "/api/v1/auth/tokens?limit=1&cursor="+*page.NextCursor, nil, "", "")
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ID == first || page.NextCursor != nil {
		t.Fatal("token pagination", code)
	}
	if _, e := s.pool.Exec(ctx, `UPDATE nmo_api_tokens SET expires_at=now()-interval '1 second' WHERE id=$1`, writeOnly.ID); e != nil {
		t.Fatal(e)
	}
	if code, _ = request("GET", "/api/v1/auth/me", nil, writeOnly.Token, ""); code != 401 {
		t.Fatal("expired token accepted", code)
	}
	// Another identity cannot list or revoke the first user's credentials.
	otherID, e := eventID()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.pool.Exec(ctx, `INSERT INTO nmo_users(id,username,password_hash,role) SELECT $1,$2,password_hash,'viewer' FROM nmo_users WHERE id=$3`, otherID, "other-token-owner", session.UserID); e != nil {
		t.Fatal(e)
	}
	raw, e := auth.Token()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.pool.Exec(ctx, `INSERT INTO nmo_browser_sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`, auth.HashToken(raw), otherID, time.Now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	other, e := auth.New(s).IssueToken(ctx, raw, auth.APITokenCreate{Name: "other-reader"})
	if e != nil {
		t.Fatal(e)
	}
	if code, _ = request("DELETE", "/api/v1/auth/tokens/"+issued.ID, nil, other.Token, ""); code != 404 {
		t.Fatal("foreign token revoked", code)
	}
	code, data = request("GET", "/api/v1/auth/tokens", nil, other.Token, "")
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ID != other.ID {
		t.Fatal("foreign metadata exposed", code)
	}
	if _, e = s.pool.Exec(ctx, `UPDATE nmo_users SET disabled=true WHERE id=$1`, otherID); e != nil {
		t.Fatal(e)
	}
	if code, _ = request("GET", "/api/v1/auth/me", nil, other.Token, ""); code != 401 {
		t.Fatal("disabled owner token accepted", code)
	}
	if code, _ = request("DELETE", "/api/v1/auth/tokens/"+issued.ID, nil, issued.Token, ""); code != 204 {
		t.Fatal("PAT self revoke", code)
	}
	if code, _ = request("GET", "/api/v1/auth/me", nil, issued.Token, ""); code != 401 {
		t.Fatal("revoked token accepted", code)
	}
	if code, _ = request("DELETE", "/api/v1/auth/tokens/"+issued.ID, nil, "", session.CSRFToken); code != 204 {
		t.Fatal("revocation not idempotent", code)
	}
	if _, e = s.pool.Exec(ctx, `DELETE FROM nmo_users WHERE id=$1`, otherID); e != nil {
		t.Fatal(e)
	}
}
