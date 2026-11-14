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
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var ErrCredentials = errors.New("invalid credentials or session")
var ErrBusy = errors.New("authentication admission limit")

const SessionTTL = 8 * time.Hour
const prefix = "$argon2id$v=19$m=65536,t=3,p=1$"

type User struct {
	ID, Username, Role, PasswordHash string
	Disabled                         bool
}
type SessionView struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expiresAt"`
	CSRFToken string    `json:"csrfToken"`
	Scopes    []string  `json:"scopes"`
}
type Repository interface {
	FindUser(context.Context, string) (User, bool, error)
	FinishLogin(context.Context, User, bool, []byte, time.Time) (bool, error)
	SessionUser(context.Context, []byte) (User, time.Time, bool, error)
	RevokeSession(context.Context, []byte) error
}
type Service struct {
	repo      Repository
	loginSlot chan struct{}
	readSlot  chan struct{}
	mu        sync.Mutex
	window    time.Time
	attempts  int
}

func New(repo Repository) *Service {
	return &Service{repo: repo, loginSlot: make(chan struct{}, 1), readSlot: make(chan struct{}, 1)}
}
func ValidUsername(s string) bool {
	if !utf8.ValidString(s) || len(s) == 0 || len(s) > 64 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func HashPassword(password string) (string, error) {
	if len(password) < 16 || len(password) > 256 || !utf8.ValidString(password) {
		return "", fmt.Errorf("password requires 16..256 UTF-8 bytes")
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
	return SessionView{u.ID, u.Username, u.Role, expiry, CSRF(raw), Scopes(u.Role)}
}
func (s *Service) Login(ctx context.Context, username, password string) (SessionView, string, error) {
	var empty SessionView
	if !ValidUsername(username) || len(password) == 0 || len(password) > 256 || !utf8.ValidString(password) {
		return empty, "", ErrCredentials
	}
	select {
	case s.loginSlot <- struct{}{}:
		defer func() { <-s.loginSlot }()
	default:
		return empty, "", ErrBusy
	}
	s.mu.Lock()
	if time.Since(s.window) >= time.Minute {
		s.window = time.Now()
		s.attempts = 0
	}
	s.attempts++
	limited := s.attempts > 20
	s.mu.Unlock()
	if limited {
		return empty, "", ErrBusy
	}
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
