package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrInput = errors.New("invalid token request")
var ErrForbidden = errors.New("insufficient permission")
var ErrNotFound = errors.New("token not found")

const APITokenPrefix = "nmo_pat_"

type APITokenCreate struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt"`
}
type APITokenView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt"`
}
type APITokenIssued struct {
	APITokenView
	Token string `json:"token"`
}
type APITokenPage struct {
	Items      []APITokenView `json:"items"`
	NextCursor *string        `json:"nextCursor"`
}

func AllowedScopes(role string, requested []string) bool {
	allowed := Scopes(role)
	for _, scope := range requested {
		if !slices.Contains(allowed, scope) {
			return false
		}
	}
	return len(allowed) > 0
}
func (s *Service) tokenAdmission() bool {
	select {
	case s.tokenSlot <- struct{}{}:
		return true
	default:
		return false
	}
}
func (s *Service) IssueToken(ctx context.Context, session string, in APITokenCreate) (APITokenIssued, error) {
	var empty APITokenIssued
	if len(in.Name) == 0 || len(in.Name) > 128 || !utf8.ValidString(in.Name) || strings.TrimSpace(in.Name) != in.Name || strings.ContainsFunc(in.Name, unicode.IsControl) {
		return empty, ErrInput
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		return empty, ErrInput
	}
	if in.Scopes == nil {
		in.Scopes = []string{"resources:read"}
	}
	if len(in.Scopes) > 9 {
		return empty, ErrInput
	}
	seen := map[string]bool{}
	for _, scope := range in.Scopes {
		if seen[scope] {
			return empty, ErrInput
		}
		seen[scope] = true
	}
	principal, err := s.Me(ctx, session)
	if err != nil {
		return empty, err
	}
	if !AllowedScopes(principal.Role, in.Scopes) {
		return empty, ErrForbidden
	}
	if !s.tokenAdmission() {
		return empty, ErrBusy
	}
	defer func() { <-s.tokenSlot }()
	secret, err := Token()
	if err != nil {
		return empty, err
	}
	raw := APITokenPrefix + secret
	// Repository atomically rechecks the current role and session before issuing.
	metadata, err := s.repo.IssueAPIToken(ctx, principal.UserID, HashToken(session), in, HashToken(raw))
	if err != nil {
		return empty, err
	}
	return APITokenIssued{metadata, raw}, nil
}
func (s *Service) TokenPrincipal(ctx context.Context, raw string) (SessionView, error) {
	if !strings.HasPrefix(raw, APITokenPrefix) || len(raw) != len(APITokenPrefix)+43 {
		return SessionView{}, ErrCredentials
	}
	if !s.tokenAdmission() {
		return SessionView{}, ErrBusy
	}
	defer func() { <-s.tokenSlot }()
	u, granted, expiry, found, err := s.repo.APITokenUser(ctx, HashToken(raw))
	if err != nil {
		return SessionView{}, err
	}
	if !found || u.Disabled || len(Scopes(u.Role)) == 0 {
		return SessionView{}, ErrCredentials
	}
	effective := []string{}
	for _, scope := range granted {
		if slices.Contains(Scopes(u.Role), scope) {
			effective = append(effective, scope)
		}
	}
	return SessionView{UserID: u.ID, Username: u.Username, Role: u.Role, ExpiresAt: expiry, Scopes: effective}, nil
}
func (s *Service) Tokens(ctx context.Context, owner, before string, limit int) (APITokenPage, error) {
	if owner == "" || limit < 1 || limit > 200 {
		return APITokenPage{}, ErrInput
	}
	if !s.tokenAdmission() {
		return APITokenPage{}, ErrBusy
	}
	defer func() { <-s.tokenSlot }()
	rows, err := s.repo.ListAPITokens(ctx, owner, before, limit)
	if err != nil {
		return APITokenPage{}, err
	}
	page := APITokenPage{Items: rows}
	if len(rows) > limit {
		cursor := rows[limit-1].ID
		page.NextCursor = &cursor
		page.Items = rows[:limit]
	}
	return page, nil
}
func (s *Service) RevokeToken(ctx context.Context, owner, id string) error {
	if owner == "" || id == "" || len(id) > 128 {
		return ErrInput
	}
	if !s.tokenAdmission() {
		return ErrBusy
	}
	defer func() { <-s.tokenSlot }()
	found, err := s.repo.RevokeAPIToken(ctx, owner, id)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}
