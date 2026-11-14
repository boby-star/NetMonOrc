package auth

import (
	"context"
	"errors"
	"slices"
	"time"
)

var ErrConflict = errors.New("identity conflict or last active administrator")
var ErrPrecondition = errors.New("identity version changed")

type UserCreate struct {
	Username        string `json:"username"`
	Role            string `json:"role"`
	InitialPassword string `json:"initialPassword"`
}
type UserPatch struct {
	Role     *string
	Disabled *bool
}
type UserView struct {
	ID        string     `json:"id"`
	Username  string     `json:"username"`
	Role      string     `json:"role"`
	Disabled  bool       `json:"disabled"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	Version   int64      `json:"-"`
}
type UserPage struct {
	Items      []UserView `json:"items"`
	NextCursor *string    `json:"nextCursor"`
}

// AdminCredential retains the specific authenticated credential for transaction
// revalidation; the HTTP principal alone can be obsolete after concurrent revoke.
type AdminCredential struct {
	UserID string
	Hash   []byte
	PAT    bool
}

func IsAdmin(principal SessionView) bool {
	return principal.Role == "admin" && slices.Contains(principal.Scopes, "system:admin")
}
func (s *Service) userAdmission() bool {
	select {
	case s.userSlot <- struct{}{}:
		return true
	default:
		return false
	}
}
func (s *Service) Users(ctx context.Context, principal SessionView, before string, limit int) (UserPage, error) {
	if !IsAdmin(principal) {
		return UserPage{}, ErrForbidden
	}
	if limit < 1 || limit > 200 || len(before) > 128 {
		return UserPage{}, ErrInput
	}
	if !s.userAdmission() {
		return UserPage{}, ErrBusy
	}
	defer func() { <-s.userSlot }()
	rows, err := s.repo.ListUsers(ctx, before, limit)
	if err != nil {
		return UserPage{}, err
	}
	page := UserPage{Items: rows}
	if len(rows) > limit {
		cursor := rows[limit-1].ID
		page.NextCursor = &cursor
		page.Items = rows[:limit]
	}
	return page, nil
}
func (s *Service) User(ctx context.Context, principal SessionView, id string) (UserView, error) {
	if !IsAdmin(principal) {
		return UserView{}, ErrForbidden
	}
	if id == "" || len(id) > 128 {
		return UserView{}, ErrInput
	}
	if !s.userAdmission() {
		return UserView{}, ErrBusy
	}
	defer func() { <-s.userSlot }()
	view, found, err := s.repo.GetUser(ctx, id)
	if err != nil {
		return view, err
	}
	if !found {
		return view, ErrNotFound
	}
	return view, nil
}
func (s *Service) CreateUser(ctx context.Context, actor AdminCredential, in UserCreate) (UserView, error) {
	if !ValidUsername(in.Username) || (in.Role != "admin" && in.Role != "viewer") {
		return UserView{}, ErrInput
	}
	if !s.userAdmission() {
		return UserView{}, ErrBusy
	}
	defer func() { <-s.userSlot }()
	if !s.admitPassword() {
		return UserView{}, ErrBusy
	}
	defer func() { <-s.loginSlot }()
	hash, err := HashPassword(in.InitialPassword)
	in.InitialPassword = ""
	if err != nil {
		return UserView{}, err
	}
	return s.repo.CreateUser(ctx, actor, in, hash)
}
func (s *Service) PatchUser(ctx context.Context, actor AdminCredential, id string, expected int64, patch UserPatch) (UserView, error) {
	if id == "" || len(id) > 128 || expected < 1 || (patch.Role == nil && patch.Disabled == nil) {
		return UserView{}, ErrInput
	}
	if patch.Role != nil && *patch.Role != "admin" && *patch.Role != "viewer" {
		return UserView{}, ErrInput
	}
	if !s.userAdmission() {
		return UserView{}, ErrBusy
	}
	defer func() { <-s.userSlot }()
	return s.repo.PatchUser(ctx, actor, id, expected, patch)
}
