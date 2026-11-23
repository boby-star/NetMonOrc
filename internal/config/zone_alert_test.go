package config

import (
	"strings"
	"testing"
)

func TestZoneSelectorAlertContract(t *testing.T) {
	source := strings.Replace(alertYAML, "  monitorRef: research-web", "  selector: {matchLabels: {environment: research}}\n  scope: zone", 1)
	source = strings.Replace(source, `monitor.state == "down"`, `zone.state == "down"`, 1)
	rule, err := CompileAlert([]byte(source))
	if err != nil || rule.Scope != "zone" || rule.ResolveWhen != `zone.state == "healthy"` || rule.Selector == nil {
		t.Fatal(rule, err)
	}
	for _, bad := range []string{source + "  monitorRef: research-web\n", strings.Replace(source, "selector:", "unknownSelector:", 1), strings.Replace(source, `zone.state == "down"`, `zone.sttae == "down"`, 1)} {
		if _, err := CompileAlert([]byte(bad)); err == nil {
			t.Fatal("invalid zone rule accepted")
		}
	}
}
