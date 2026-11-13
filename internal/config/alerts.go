package config

import (
	"bytes"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"gopkg.in/yaml.v3"
	"io"
	"net/netip"
	"reflect"
	"sort"
	"time"
)

func decodeResource(b []byte, v any) error {
	if len(b) > 1<<20 {
		return fmt.Errorf("resource exceeds 1 MiB")
	}
	var tree yaml.Node
	if e := yaml.Unmarshal(b, &tree); e != nil {
		return e
	}
	if len(tree.Content) != 1 {
		return fmt.Errorf("one resource required")
	}
	if e := scalarTypes(tree.Content[0], reflect.TypeOf(v).Elem()); e != nil {
		return e
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if e := d.Decode(v); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return fmt.Errorf("one resource document required")
	}
	return nil
}
func nonnegative(s *string, fallback time.Duration) (time.Duration, error) {
	if s == nil {
		return fallback, nil
	}
	if !durationPattern.MatchString(*s) {
		return 0, fmt.Errorf("invalid duration")
	}
	d, e := time.ParseDuration(*s)
	if e != nil || d < 0 {
		return 0, fmt.Errorf("invalid nonnegative duration")
	}
	return d, nil
}
func CompileAlert(b []byte) (alerting.Rule, error) {
	var m struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Monitor     string   `yaml:"monitorRef"`
			Scope       string   `yaml:"scope"`
			When        string   `yaml:"when"`
			Resolve     string   `yaml:"resolveWhen"`
			Severity    string   `yaml:"severity"`
			For         *string  `yaml:"for"`
			Cooldown    *string  `yaml:"cooldown"`
			Channel     string   `yaml:"channelRef"`
			Events      []string `yaml:"notifyEvents"`
			Enabled     *bool    `yaml:"enabled"`
			Description string   `yaml:"description"`
		} `yaml:"spec"`
	}
	r := alerting.Rule{}
	if e := decodeResource(b, &m); e != nil {
		return r, e
	}
	if m.APIVersion != "netmonorc.io/v1alpha1" || m.Kind != "AlertRule" || !namePattern.MatchString(m.Metadata.Name) || !namePattern.MatchString(m.Spec.Monitor) {
		return r, fmt.Errorf("invalid AlertRule envelope/monitorRef")
	}
	s := m.Spec
	r = alerting.Rule{Name: m.Metadata.Name, Monitor: s.Monitor, Scope: s.Scope, When: s.When, ResolveWhen: s.Resolve, Severity: s.Severity, Channel: s.Channel, NotifyEvents: s.Events, Enabled: true, Description: s.Description}
	if r.Scope == "" {
		r.Scope = "monitor"
	}
	if r.Scope != "monitor" {
		return r, fmt.Errorf("research alerting supports monitor scope only")
	}
	if r.ResolveWhen == "" {
		r.ResolveWhen = `monitor.state == "healthy"`
	}
	if r.Severity == "" {
		r.Severity = "warning"
	}
	if r.Severity != "info" && r.Severity != "warning" && r.Severity != "critical" {
		return r, fmt.Errorf("invalid severity")
	}
	if r.Channel == "" {
		r.Channel = "default-webhook"
	}
	if !namePattern.MatchString(r.Channel) || len(r.Description) > 1024 {
		return r, fmt.Errorf("invalid alert channel/description")
	}
	if s.Enabled != nil {
		r.Enabled = *s.Enabled
	}
	if r.NotifyEvents == nil {
		r.NotifyEvents = []string{"firing", "resolved"}
	}
	seen := map[string]bool{}
	for _, kind := range r.NotifyEvents {
		if seen[kind] || (kind != "firing" && kind != "resolved") {
			return r, fmt.Errorf("invalid notifyEvents")
		}
		seen[kind] = true
	}
	sort.Strings(r.NotifyEvents)
	var e error
	r.For, e = nonnegative(s.For, 0)
	if e != nil {
		return r, e
	}
	r.Cooldown, e = nonnegative(s.Cooldown, 5*time.Minute)
	if e != nil {
		return r, e
	}
	_, e = alerting.New(r)
	return r, e
}
func CompileWebhook(b []byte) (webhook.Endpoint, error) {
	var m struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			URL     string `yaml:"url"`
			Enabled *bool  `yaml:"enabled"`
			Timeout string `yaml:"timeout"`
			Auth    struct {
				Type string `yaml:"type"`
				Ref  string `yaml:"secretRef"`
			} `yaml:"auth"`
			Retry struct {
				Max        *int   `yaml:"maxAttempts"`
				Initial    string `yaml:"initialBackoff"`
				MaxBackoff string `yaml:"maxBackoff"`
			} `yaml:"retry"`
		} `yaml:"spec"`
	}
	r := webhook.Endpoint{}
	if e := decodeResource(b, &m); e != nil {
		return r, e
	}
	if m.APIVersion != "netmonorc.io/v1alpha1" || m.Kind != "WebhookEndpoint" || !namePattern.MatchString(m.Metadata.Name) {
		return r, fmt.Errorf("invalid WebhookEndpoint envelope")
	}
	s := m.Spec
	r = webhook.Endpoint{Name: m.Metadata.Name, URL: s.URL, Enabled: true, AuthType: s.Auth.Type, SecretRef: s.Auth.Ref, MaxAttempts: 5}
	if s.Enabled != nil {
		r.Enabled = *s.Enabled
	}
	if r.AuthType == "" {
		r.AuthType = "none"
	}
	if (r.AuthType == "none" && r.SecretRef != "") || (r.AuthType == "bearer" && !namePattern.MatchString(r.SecretRef)) || (r.AuthType != "none" && r.AuthType != "bearer") {
		return r, fmt.Errorf("invalid webhook auth/secretRef")
	}
	if s.Retry.Max != nil {
		r.MaxAttempts = *s.Retry.Max
	}
	var e error
	r.Timeout, e = duration(s.Timeout, 5*time.Second)
	if e != nil {
		return r, e
	}
	r.InitialBackoff, e = duration(s.Retry.Initial, 2*time.Second)
	if e != nil {
		return r, e
	}
	r.MaxBackoff, e = duration(s.Retry.MaxBackoff, time.Minute)
	if e != nil {
		return r, e
	}
	if r.Timeout < time.Second || r.Timeout > 30*time.Second || r.MaxAttempts < 1 || r.MaxAttempts > 10 || r.InitialBackoff < time.Second || r.InitialBackoff > 5*time.Minute || r.MaxBackoff < time.Second || r.MaxBackoff > 30*time.Minute {
		return r, fmt.Errorf("invalid webhook timeout/retry limits")
	}
	e = webhook.Allowed(r.URL, []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")})
	return r, e
}
