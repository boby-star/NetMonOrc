package agent

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/boby-star/NetMonOrc/internal/durable"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"golang.org/x/sys/unix"
)

const MaxPending = 1000

type Spool struct {
	mu         sync.Mutex
	dir, agent string
	items      map[string]monitor.Observation
	lock       *os.File
	reserved   int
}

func OpenSpool(dir, agent string) (*Spool, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("Agent data directory must be a private directory")
	}
	f, e := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("Agent data directory is already locked")
	}
	s := &Spool{dir: dir, agent: agent, items: map[string]monitor.Observation{}, lock: f}
	identity := filepath.Join(dir, "identity")
	b, e := os.ReadFile(identity)
	if os.IsNotExist(e) {
		e = durable.AtomicWrite(identity, []byte(agent))
	} else if e == nil && string(b) != agent {
		e = fmt.Errorf("data directory belongs to a different Agent")
	}
	if e != nil {
		s.Close()
		return nil, e
	}
	files, e := os.ReadDir(dir)
	if e != nil {
		s.Close()
		return nil, e
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".pending-") {
			if e = os.Remove(filepath.Join(dir, f.Name())); e != nil {
				s.Close()
				return nil, e
			}
			continue
		}
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		if len(s.items) >= MaxPending {
			s.Close()
			return nil, fmt.Errorf("spool capacity exceeded")
		}
		info, e := f.Info()
		if e != nil || info.Size() > 8192 || !info.Mode().IsRegular() {
			s.Close()
			return nil, fmt.Errorf("invalid spool file")
		}
		b, e = os.ReadFile(filepath.Join(dir, f.Name()))
		if e != nil {
			s.Close()
			return nil, e
		}
		var o monitor.Observation
		if e = json.Unmarshal(b, &o); e != nil || !validID(o.ID) || o.Agent != agent || o.ID+".json" != f.Name() {
			s.Close()
			return nil, fmt.Errorf("invalid spool record")
		}
		s.items[o.ID] = o
	}
	return s, nil
}
func (s *Spool) Close() error { return s.lock.Close() }
func (s *Spool) Full() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)+s.reserved >= MaxPending
}
func (s *Spool) Reserve() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items)+s.reserved >= MaxPending {
		return false
	}
	s.reserved++
	return true
}
func (s *Spool) Release() { s.mu.Lock(); defer s.mu.Unlock(); s.reserved-- }
func (s *Spool) AddReserved(o monitor.Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reserved < 1 {
		return fmt.Errorf("no reserved spool slot")
	}
	s.reserved--
	return s.add(o)
}
func (s *Spool) Add(o monitor.Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.add(o)
}
func (s *Spool) add(o monitor.Observation) error {
	if len(s.items)+s.reserved >= MaxPending {
		return fmt.Errorf("spool full")
	}
	if o.Agent != s.agent || !validID(o.ID) {
		return fmt.Errorf("invalid spool identity")
	}
	if _, ok := s.items[o.ID]; ok {
		return fmt.Errorf("duplicate local observation ID")
	}
	b, e := json.Marshal(o)
	if e != nil {
		return e
	}
	if len(b) > 8192 {
		return fmt.Errorf("spool record too large")
	}
	if e = durable.AtomicWrite(filepath.Join(s.dir, o.ID+".json"), b); e != nil {
		return e
	}
	s.items[o.ID] = o
	return nil
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, e := hex.DecodeString(id)
	return e == nil
}
func (s *Spool) Pending() []monitor.Observation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]monitor.Observation, 0, len(s.items))
	for _, o := range s.items {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FinishedAt.Equal(out[j].FinishedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].FinishedAt.Before(out[j].FinishedAt)
	})
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
func (s *Spool) Ack(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if _, ok := s.items[id]; !ok {
			return fmt.Errorf("ACK refers to unknown observation")
		}
		if e := os.Remove(filepath.Join(s.dir, id+".json")); e != nil {
			return e
		}
		delete(s.items, id)
	}
	return durable.SyncDir(s.dir)
}
