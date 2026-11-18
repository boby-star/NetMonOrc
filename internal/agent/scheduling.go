package agent

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"sort"
	"time"
)

type regularJob struct {
	assignment *assignment
	check      monitor.Check
	due        time.Time
}

// Priority controls dispatch, never the assessment weight. Among equally urgent
// jobs, older due times win, then Monitor/check IDs. No map iteration tie-break.
func dueRegular(assignments []*assignment, id string, next map[string]time.Time, now time.Time) []regularJob {
	var jobs []regularJob
	for _, a := range assignments {
		if !leaseValid(a) || !a.plan.RegularAllowed(id) {
			continue
		}
		for _, c := range a.plan.Checks {
			key := a.plan.Revision + "/" + c.ID
			due, known := next[key]
			if !known && c.Jitter > 0 {
				due = now.Add(scheduleJitter(id, key, 0, c.Jitter))
				next[key] = due
			}
			if !now.Before(due) {
				jobs = append(jobs, regularJob{a, c, due})
			}
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		a, b := jobs[i], jobs[j]
		ap, bp := a.check.Priority, b.check.Priority
		if ap == 0 {
			ap = 50
		}
		if bp == 0 {
			bp = 50
		}
		if ap != bp {
			return ap > bp
		}
		if !a.due.Equal(b.due) {
			return a.due.Before(b.due)
		}
		if a.assignment.plan.Name != b.assignment.plan.Name {
			return a.assignment.plan.Name < b.assignment.plan.Name
		}
		return a.check.ID < b.check.ID
	})
	return jobs
}

// A stable pseudo-random offset in [0,limit], reproducible from source,
// revision, check and activation ordinal; no random seed is hidden in a plan.
func scheduleJitter(agent, key string, ordinal uint64, limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%d", agent, key, ordinal)))
	return time.Duration(binary.BigEndian.Uint64(digest[:8]) % uint64(limit+1))
}
