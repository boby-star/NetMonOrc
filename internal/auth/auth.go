// Package auth implements password and browser-session logic independently of HTTP/SQL.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var ErrCredentials = errors.New("invalid credentials or session")
var ErrPassword = errors.New("password requires 16..256 UTF-8 bytes")
var ErrBusy = errors.New("authentication admission limit")

const SessionTTL = 8 * time.Hour
const prefix = "$argon2id$v=19$m=65536,t=3,p=1$"

type User struct {
	ID, Username, Role, PasswordHash string
	Disabled                         bool
}
type SessionView struct {
	UserID    string     `json:"userId"`
	Username  string     `json:"username"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expiresAt"`
	CSRFToken string     `json:"csrfToken,omitempty"`
	Scopes    []string   `json:"scopes"`
}
type Repository interface {
	FindUser(context.Context, string) (User, bool, error)
	FinishLogin(context.Context, User, bool, []byte, time.Time) (bool, error)
	SessionUser(context.Context, []byte) (User, time.Time, bool, error)
	RevokeSession(context.Context, []byte) error
	IssueAPIToken(context.Context, string, []byte, APITokenCreate, []byte) (APITokenView, error)
	ListAPITokens(context.Context, string, string, int) ([]APITokenView, error)
	RevokeAPIToken(context.Context, string, string) (bool, error)
	APITokenUser(context.Context, []byte) (User, []string, *time.Time, bool, error)
	ChangePassword(context.Context, User, []byte, bool, string, []byte, time.Time) (bool, error)
	ListUsers(context.Context, string, int) ([]UserView, error)
	GetUser(context.Context, string) (UserView, bool, error)
	CreateUser(context.Context, AdminCredential, UserCreate, string) (UserView, error)
	PatchUser(context.Context, AdminCredential, string, int64, UserPatch) (UserView, error)
}
type Service struct {
	repo      Repository
	loginSlot chan struct{}
	readSlot  chan struct{}
	tokenSlot chan struct{}
	userSlot  chan struct{}
	mu        sync.Mutex
	window    time.Time
	attempts  int
}

func New(repo Repository) *Service {
	return &Service{repo: repo, loginSlot: make(chan struct{}, 1), readSlot: make(chan struct{}, 1), tokenSlot: make(chan struct{}, 1), userSlot: make(chan struct{}, 1)}
}
func ValidUsername(s string) bool {
	if !utf8.ValidString(s) || len(s) == 0 || len(s) > 64 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func HashPassword(password string) (string, error) {
	if len(password) < 16 || len(password) > 256 || !utf8.ValidString(password) {
		return "", ErrPassword
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return prefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}
func VerifyPassword(password, hash string) (bool, error) {
	if !strings.HasPrefix(hash, prefix) || len(hash) > 200 {
		return false, fmt.Errorf("unsupported stored password hash")
	}
	parts := strings.Split(strings.TrimPrefix(hash, prefix), "$")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid password hash")
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[0])
	if e != nil || len(salt) != 16 {
		return false, fmt.Errorf("invalid salt")
	}
	expected, e := base64.RawStdEncoding.DecodeString(parts[1])
	if e != nil || len(expected) != 32 {
		return false, fmt.Errorf("invalid password digest")
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
func Token() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func HashToken(raw string) []byte { sum := sha256.Sum256([]byte(raw)); return sum[:] }
func CSRF(raw string) string {
	mac := hmac.New(sha256.New, []byte(raw))
	mac.Write([]byte("netmonorc-browser-csrf-v1"))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func Scopes(role string) []string {
	if role != "admin" && role != "viewer" {
		return nil
	}
	s := []string{"resources:read", "agents:read", "diagnostics:read", "alerts:read"}
	if role == "admin" {
		s = append(s, "resources:write", "agents:manage", "diagnostics:run", "alerts:manage", "system:admin")
	}
	return s
}
func view(u User, expiry time.Time, raw string) SessionView {
	return SessionView{u.ID, u.Username, u.Role, &expiry, CSRF(raw), Scopes(u.Role)}
}
func (s *Service) Login(ctx context.Context, username, password string) (SessionView, string, error) {
	var empty SessionView
	if !ValidUsername(username) || len(password) == 0 || len(password) > 256 || !utf8.ValidString(password) {
		return empty, "", ErrCredentials
	}
	if !s.admitPassword() {
		return empty, "", ErrBusy
	}
	defer func() { <-s.loginSlot }()
	u, found, e := s.repo.FindUser(ctx, username)
	if e != nil {
		return empty, "", e
	}
	hash := u.PasswordHash
	if !found {
		u.Username = username
		hash = prefix + base64.RawStdEncoding.EncodeToString(make([]byte, 16)) + "$" + base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	}
	correct, e := VerifyPassword(password, hash)
	if e != nil {
		return empty, "", e
	}
	raw, e := Token()
	if e != nil {
		return empty, "", e
	}
	expiry := time.Now().UTC().Add(SessionTTL)
	ok, e := s.repo.FinishLogin(ctx, u, found && correct && !u.Disabled && len(Scopes(u.Role)) > 0, HashToken(raw), expiry)
	if e != nil {
		return empty, "", e
	}
	if !ok {
		return empty, "", ErrCredentials
	}
	return view(u, expiry, raw), raw, nil
}
func (s *Service) Me(ctx context.Context, raw string) (SessionView, error) {
	if len(raw) != 43 {
		return SessionView{}, ErrCredentials
	}
	select {
	case s.readSlot <- struct{}{}:
		defer func() { <-s.readSlot }()
	default:
		return SessionView{}, ErrBusy
	}
	u, expiry, found, e := s.repo.SessionUser(ctx, HashToken(raw))
	if e != nil {
		return SessionView{}, e
	}
	if !found || u.Disabled || len(Scopes(u.Role)) == 0 {
		return SessionView{}, ErrCredentials
	}
	return view(u, expiry, raw), nil
}
func (s *Service) Logout(ctx context.Context, raw string) error {
	return s.repo.RevokeSession(ctx, HashToken(raw))
}

// admitPassword shares one bounded hashing slot and rate budget across login and changes.
func (s *Service) admitPassword() bool {
	select {
	case s.loginSlot <- struct{}{}:
	default:
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.window) >= time.Minute {
		s.window = time.Now()
		s.attempts = 0
	}
	s.attempts++
	if s.attempts > 20 {
		<-s.loginSlot
		return false
	}
	return true
}

func (s *Service) ChangePassword(ctx context.Context, raw, currentPassword, newPassword string) (SessionView, string, error) {
	var empty SessionView
	if len(newPassword) < 16 || len(newPassword) > 256 || !utf8.ValidString(newPassword) {
		return empty, "", ErrPassword
	}
	if len(currentPassword) == 0 || len(currentPassword) > 256 || !utf8.ValidString(currentPassword) {
		return empty, "", ErrCredentials
	}
	if !s.admitPassword() {
		return empty, "", ErrBusy
	}
	defer func() { <-s.loginSlot }()
	v, err := s.Me(ctx, raw)
	if err != nil {
		return empty, "", err
	}
	u, found, err := s.repo.FindUser(ctx, v.Username)
	if err != nil {
		return empty, "", err
	}
	if !found || u.ID != v.UserID {
		return empty, "", ErrCredentials
	}
	correct, err := VerifyPassword(currentPassword, u.PasswordHash)
	if err != nil {
		return empty, "", err
	}
	newHash := ""
	if correct {
		newHash, err = HashPassword(newPassword)
		if err != nil {
			return empty, "", err
		}
	}
	fresh, err := Token()
	if err != nil {
		return empty, "", err
	}
	expiry := time.Now().UTC().Add(SessionTTL)
	ok, err := s.repo.ChangePassword(ctx, u, HashToken(raw), correct, newHash, HashToken(fresh), expiry)
	if err != nil {
		return empty, "", err
	}
	if !ok {
		return empty, "", ErrCredentials
	}
	return view(u, expiry, fresh), fresh, nil
}
