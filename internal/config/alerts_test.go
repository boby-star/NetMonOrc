package config

import (
	"strings"
	"testing"
	"time"
)

const alertYAML = `apiVersion: netmonorc.io/v1alpha1
kind: AlertRule
metadata: {name: service-outage}
spec:
  monitorRef: research-web
  when: monitor.state == "down"
`
const endpointYAML = `apiVersion: netmonorc.io/v1alpha1
kind: WebhookEndpoint
metadata: {name: default-webhook}
spec:
  url: https://192.0.2.1/events
`

func TestAlertContractDefaultsAndRejections(t *testing.T) {
	r, err := CompileAlert([]byte(alertYAML))
	if err != nil || r.For != 0 || r.Cooldown != 5*time.Minute || r.Scope != "monitor" || !r.Enabled || len(r.NotifyEvents) != 2 {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	for _, fields := range []string{"  scope: zone\n", "  for: -1s\n", "  cooldown: 123\n", "  notifyEvents: [firing, firing]\n", "  severity: fatal\n", "  unknown: value\n"} {
		if _, err := CompileAlert([]byte(alertYAML + fields)); err == nil {
			t.Errorf("accepted %s", fields)
		}
	}
	if _, err := CompileAlert([]byte(strings.Replace(alertYAML, `monitor.state == "down"`, `monitor.state`, 1))); err == nil {
		t.Fatal("non-bool expression accepted")
	}
	r, err = CompileAlert([]byte(alertYAML + "  notifyEvents: []\n"))
	if err != nil || len(r.NotifyEvents) != 0 {
		t.Fatal("explicit empty events must disable delivery")
	}
}
func TestWebhookContractBounds(t *testing.T) {
	r, err := CompileWebhook([]byte(endpointYAML))
	if err != nil || r.Timeout != 5*time.Second || r.MaxAttempts != 5 || r.AuthType != "none" {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	for _, fields := range []string{"  timeout: 31s\n", "  retry: {maxAttempts: 11}\n", "  retry: {initialBackoff: 0s}\n", "  auth: {type: bearer}\n", "  auth: {secretRef: secret}\n"} {
		if _, err := CompileWebhook([]byte(endpointYAML + fields)); err == nil {
			t.Errorf("accepted %s", fields)
		}
	}
	if _, err := CompileWebhook([]byte(endpointYAML + "  retry: {initialBackoff: 5s, maxBackoff: 1s}\n")); err != nil {
		t.Fatal("maxBackoff should cap initial delay", err)
	}
}
