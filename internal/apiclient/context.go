// Package apiclient holds the CLI's private authenticated context and TLS client.
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"golang.org/x/sys/unix"
)

type Context struct {
	Server string `json:"server"`
	Token  string `json:"token"`
	CAFile string `json:"caFile,omitempty"`
}

func (c Context) Validate() error {
	u, err := url.Parse(c.Server)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.RawPath != "" || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("server must be an HTTPS origin without credentials, path or query")
	}
	if !strings.HasPrefix(c.Token, auth.APITokenPrefix) || len(c.Token) != len(auth.APITokenPrefix)+43 || strings.ContainsAny(c.Token, " \t\r\n") {
		return fmt.Errorf("personal API token required")
	}
	return nil
}
func privateParent(path string, create bool) error {
	parent := filepath.Dir(path)
	if create {
		if err := os.MkdirAll(parent, 0700); err != nil {
			return fmt.Errorf("context directory unavailable")
		}
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("context directory must be private (0700) and not a symlink")
	}
	return nil
}
func Load(path string) (Context, error) {
	var c Context
	if err := privateParent(path, false); err != nil {
		return c, err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return c, fmt.Errorf("private CLI context unavailable; login first")
	}
	file := os.NewFile(uintptr(fd), "CLI context")
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return c, fmt.Errorf("CLI context must be a private regular file (0600)")
	}
	b, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(b) > 16384 {
		return c, fmt.Errorf("invalid CLI context")
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return Context{}, fmt.Errorf("invalid CLI context")
	}
	if err = c.Validate(); err != nil {
		return Context{}, err
	}
	return c, nil
}
func Save(path string, c Context) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := privateParent(path, true); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("existing context must be a private regular file")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("existing context unavailable")
	}
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("cannot encode CLI context")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".context-*")
	if err != nil {
		return fmt.Errorf("cannot create private context")
	}
	temp := file.Name()
	defer os.Remove(temp)
	if _, err = file.Write(append(b, '\n')); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return fmt.Errorf("cannot save private context")
	}
	if err = os.Rename(temp, path); err != nil {
		return fmt.Errorf("cannot replace private context")
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("cannot sync context directory")
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return fmt.Errorf("cannot sync context directory")
	}
	return nil
}
func Logout(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	if _, err := Load(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cannot remove private context")
	}
	return nil
}
func (c Context) WhoAmI(ctx context.Context) (auth.SessionView, error) {
	var view auth.SessionView
	client, err := c.Client()
	if err != nil {
		return view, err
	}
	defer client.Close()
	b, err := client.JSON(ctx, "GET", "/api/v1/auth/me", nil)
	if err != nil {
		return view, err
	}
	if len(b) > 16384 {
		return view, fmt.Errorf("identity response too large")
	}
	if err = json.Unmarshal(b, &view); err != nil || view.UserID == "" || (view.Role != "admin" && view.Role != "viewer") || view.CSRFToken != "" {
		return auth.SessionView{}, fmt.Errorf("invalid API token identity response")
	}
	return view, nil
}
