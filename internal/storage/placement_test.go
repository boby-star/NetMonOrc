package storage

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

func TestPostgresDynamicPlacementPersistence(t *testing.T) {
	file := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if file == "" {
		t.Skip("requires real PostgreSQL DSN file")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root, err := Open(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := pgx.Identifier{"dynamic_" + newID()}.Sanitize()
	if _, err = root.pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer root.pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	dsn, err := url.Parse(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", strings.Trim(schema, `"`))
	dsn.RawQuery = query.Encode()
	store, err := Open(ctx, dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	base := resources.Registry{ExecuteAll: true, Zones: []string{"office", "server"}, Roster: []monitor.Slot{{Agent: "branch", Zone: "office"}, {Agent: "dc", Zone: "server"}}}
	policy := resources.Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "PlacementPolicy", Metadata: resources.Metadata{Name: "rotation"}, Spec: json.RawMessage(`{"strategy":"roundRobin","replicas":1}`)}
	if _, err = store.SeedResource(ctx, policy, "", base); err != nil {
		t.Fatal(err)
	}
	versions := map[string]string{}
	for _, name := range []string{"one", "two"} {
		m := resources.Manifest{APIVersion: policy.APIVersion, Kind: "Monitor", Metadata: resources.Metadata{Name: name}, Spec: json.RawMessage(`{"target":{"address":"198.18.0.80"},"placementRef":"rotation","checks":[{"id":"http","type":"http","runner":"native-http","interval":"10s","timeout":"1s","config":{"url":"http://198.18.0.80/"}}]}`)}
		v, e := store.SeedResource(ctx, m, "", base)
		if e != nil {
			t.Fatal(e)
		}
		versions[name] = v.ResourceVersion
	}
	now := time.Now()
	expiry := now.Add(time.Hour)
	capacity, active := 2, 0
	var agents []agentstatus.View
	for _, slot := range base.Roster {
		agents = append(agents, agentstatus.View{AgentID: slot.Agent, NetworkZone: slot.Zone, OnlineState: "online", LastHeartbeat: &now, CertExpiresAt: &expiry, MaxConcurrentProbes: &capacity, ActiveProbes: &active, Capabilities: []string{"http", "native-http", "placement:dynamic", "target-cidr:198.18.0.0/24"}})
	}
	changed, err := store.ReconcilePlacement(ctx, base, agents, now)
	if err != nil || !changed {
		t.Fatal("initial selection", changed, err)
	}
	_, compiled, err := store.LoadRuntime(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	one, two := compiled["Monitor/one"].Monitor, compiled["Monitor/two"].Monitor
	if one.FixedPlacement.Selected[0].Agent != "branch" || two.FixedPlacement.Selected[0].Agent != "dc" {
		t.Fatal("shared policy cursor not advanced", one.FixedPlacement, two.FixedPlacement)
	}
	if one.Zones[0] != "office" || two.Zones[0] != "server" {
		t.Fatal("default assessment zones not resolved from selection")
	}
	for name, version := range versions {
		v, _, e := store.Resource(ctx, name)
		if e != nil || v.ResourceVersion != version {
			t.Fatal("placement changed source resourceVersion", name, e)
		}
	}
	store.Close()
	store, err = Open(ctx, dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	changed, err = store.ReconcilePlacement(ctx, base, agents, now)
	if err != nil || changed {
		t.Fatal("restart rotated a valid assignment", changed, err)
	}
	agents[0].OnlineState = "offline"
	changed, err = store.ReconcilePlacement(ctx, base, agents, now)
	if err != nil || !changed {
		t.Fatal("failover not selected", changed, err)
	}
	_, after, err := store.LoadRuntime(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if after["Monitor/one"].Monitor.FixedPlacement.Selected[0].Agent != "dc" || after["Monitor/one"].Revision == one.Revision || after["Monitor/two"].Revision != two.Revision {
		t.Fatal("failover mixed unrelated revisions")
	}
	agents[1].OnlineState = "offline"
	if _, err = store.ReconcilePlacement(ctx, base, agents, now); err != nil {
		t.Fatal(err)
	}
	_, pending, err := store.LoadRuntime(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending["Monitor/one"].Monitor.FixedPlacement.Selected) != 0 || len(pending["Monitor/two"].Monitor.FixedPlacement.Selected) != 0 {
		t.Fatal("partial assignment fabricated")
	}
}
