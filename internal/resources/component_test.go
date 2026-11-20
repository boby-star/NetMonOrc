package resources

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/config"
)

func TestComponentValidationBindingAndCanonicalRevision(t *testing.T) {
	base, policy, m := fixedPlacementFixture(t)
	base.AgentLabels = map[string]map[string]string{"branch": {"role": "probe"}, "dc": {"role": "other"}}
	c := Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "Component", Metadata: Metadata{Name: "local-blackbox"}, Spec: json.RawMessage(`{"driver":"blackbox","agentSelector":{"matchLabels":{"role":"probe"}},"endpoint":"http://blackbox:9115/"}`)}
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{m, c, policy})
	if err != nil {
		t.Fatal(err)
	}
	p := *compiled["Monitor/service"].Monitor
	if len(p.Components) != 1 || p.Components["branch"].Endpoint != "http://blackbox:9115" || !p.Components["branch"].Enabled {
		t.Fatal("trusted selector/default binding incorrect", p.Components)
	}
	if err = config.VerifyPlan(p, "office", "branch"); err != nil {
		t.Fatal(err)
	}
	c.Spec = json.RawMessage(`{"driver":"blackbox","mode":"external","agentSelector":{"matchLabels":{"role":"probe"}},"endpoint":"http://blackbox:9115","moduleSet":"standard-network","enabled":true,"description":"display only"}`)
	_, again, err := ResolveCatalog(base, nil, []Manifest{policy, c, m})
	if err != nil || again["Monitor/service"].Revision != p.Revision {
		t.Fatal("order/default/description changed execution hash", err)
	}
	c.Spec = json.RawMessage(strings.Replace(string(c.Spec), `"enabled":true`, `"enabled":false`, 1))
	_, disabled, err := ResolveCatalog(base, nil, []Manifest{c, m, policy})
	if err != nil || disabled["Monitor/service"].Revision == p.Revision || disabled["Monitor/service"].Monitor.Components["branch"].Enabled {
		t.Fatal("disabled binding not in revision", err)
	}
	duplicate := c
	duplicate.Metadata.Name = "other-exporter"
	if _, _, err = ResolveCatalog(base, nil, []Manifest{c, duplicate}); err == nil {
		t.Fatal("ambiguous Agent binding accepted without any Monitor")
	}
	for _, spec := range []string{
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://blackbox:9115","enabled":null}`,
		`{"driver":"blackbox","mode":"managed","agentSelector":{},"endpoint":"http://blackbox:9115"}`,
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://8.8.8.8:9115"}`,
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://blackbox:9115/probe"}`,
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://user:secret@blackbox:9115"}`,
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://example.com:9115"}`,
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://blackbox:9115","moduleSet":"arbitrary"}`,
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://blackbox:9115","enabled":"true"}`,
		`{"driver":"blackbox","agentSelector":{},"endpoint":"http://blackbox:9115","image":"arbitrary"}`,
	} {
		c.Spec = json.RawMessage(spec)
		if _, err = base.CompileResource(c); err == nil {
			t.Fatal("invalid Component accepted", spec)
		}
	}
}
