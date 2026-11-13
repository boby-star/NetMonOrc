package config

import (
	"bytes"
	"fmt"
	"net/netip"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"gopkg.in/yaml.v3"
)

type httpConfig struct {
	BodyMatchesRegex string `yaml:"bodyMatchesRegex"`
	URL              string `yaml:"url"`
	Method           string `yaml:"method"`
	ExpectedStatus   []int  `yaml:"expectedStatus"`
	VerifyTLS        *bool  `yaml:"verifyTLS"`
	FollowRedirects  bool   `yaml:"followRedirects"`
	MaxBodyBytes     *int64 `yaml:"maxBodyBytes"`
}
type tcpConfig struct {
	Port                int    `yaml:"port"`
	PreferredIPProtocol string `yaml:"preferredIPProtocol"`
	IPProtocolFallback  bool   `yaml:"ipProtocolFallback"`
}
type dnsConfig struct {
	Resolver            string   `yaml:"resolver"`
	QueryName           string   `yaml:"queryName"`
	RecordType          string   `yaml:"recordType"`
	Transport           string   `yaml:"transport"`
	ExpectedRcodes      []string `yaml:"expectedRcodes"`
	ExpectedRecordRegex string   `yaml:"expectedRecordRegex"`
}
type icmpConfig struct {
	PreferredIPProtocol string `yaml:"preferredIPProtocol"`
	IPProtocolFallback  bool   `yaml:"ipProtocolFallback"`
	PayloadSize         *int   `yaml:"payloadSize"`
	DontFragment        bool   `yaml:"dontFragment"`
}

type probeDecoder func(yaml.Node) (monitor.Check, error)

var decoders = map[string]probeDecoder{"http": decodeHTTP, "tcp": decodeTCP, "dns": decodeDNS, "icmp": decodeICMP}

func compileProbe(s checkSpec, target string) (monitor.Check, error) {
	decode, ok := decoders[s.Type]
	if !ok {
		return monitor.Check{}, fmt.Errorf("unsupported check type %q", s.Type)
	}
	runner := s.Runner
	if runner == "" || runner == "auto" {
		runner = "blackbox"
		if len(s.Extract) > 0 {
			runner = "native-http"
		}
	}
	if runner != "blackbox" && (runner != "native-http" || s.Type != "http") {
		return monitor.Check{}, fmt.Errorf("runner incompatible with check type")
	}
	c, e := decode(s.Config)
	if e != nil {
		return c, e
	}
	c.Type = s.Type
	c.Extract, e = compileExtract(s.Extract)
	if e != nil {
		return c, e
	}
	c.Runner = runner
	if e = validateProbe(c); e != nil {
		return c, e
	}
	if s.Type == "tcp" || s.Type == "icmp" {
		if strings.ContainsAny(target, "/ \t\r\n") || target == "" {
			return c, fmt.Errorf("invalid target.address")
		}
	}
	return c, nil
}

// KnownFields closes each protocol's schema; scalar checks prevent YAML's
// legacy yes/no booleans and float-to-integer coercion for ports or sizes.
func decodeClosed(n yaml.Node, v any) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("config must be a mapping")
	}
	if e := scalarTypes(&n, reflect.TypeOf(v).Elem()); e != nil {
		return e
	}
	b, e := yaml.Marshal(&n)
	if e != nil {
		return e
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	return d.Decode(v)
}
func scalarTypes(n *yaml.Node, t reflect.Type) error {
	if t == reflect.TypeOf(yaml.Node{}) {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return fmt.Errorf("expected mapping")
		}
		for i := 0; i < len(n.Content); i += 2 {
			for j := 0; j < t.NumField(); j++ {
				f := t.Field(j)
				if strings.Split(f.Tag.Get("yaml"), ",")[0] == n.Content[i].Value {
					if e := scalarTypes(n.Content[i+1], f.Type); e != nil {
						return fmt.Errorf("%s: %w", n.Content[i].Value, e)
					}
					break
				}
			}
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return fmt.Errorf("expected list")
		}
		for _, x := range n.Content {
			if e := scalarTypes(x, t.Elem()); e != nil {
				return e
			}
		}
	case reflect.String:
		if n.Tag != "!!str" {
			return fmt.Errorf("expected string")
		}
	case reflect.Bool:
		if n.Tag != "!!bool" {
			return fmt.Errorf("expected true/false")
		}
	case reflect.Float64:
		if n.Tag != "!!int" && n.Tag != "!!float" {
			return fmt.Errorf("expected number")
		}
	case reflect.Int, reflect.Int64:
		if n.Tag != "!!int" {
			return fmt.Errorf("expected integer")
		}
	}
	return nil
}
func decodeHTTP(n yaml.Node) (monitor.Check, error) {
	var h httpConfig
	c := monitor.Check{}
	if e := decodeClosed(n, &h); e != nil {
		return c, e
	}
	if h.FollowRedirects || (h.VerifyTLS != nil && !*h.VerifyTLS) {
		return c, fmt.Errorf("redirects and disabled TLS verification are not supported")
	}
	c.BodyMatchesRegex = h.BodyMatchesRegex
	c.URL = h.URL
	c.Method = h.Method
	if c.Method == "" {
		c.Method = "GET"
	}
	c.ExpectedStatus = h.ExpectedStatus
	if c.ExpectedStatus == nil {
		c.ExpectedStatus = []int{200}
	}
	sort.Ints(c.ExpectedStatus)
	c.MaxBodyBytes = 16384
	if h.MaxBodyBytes != nil {
		c.MaxBodyBytes = *h.MaxBodyBytes
	}
	return c, nil
}
func decodeTCP(n yaml.Node) (monitor.Check, error) {
	var v tcpConfig
	c := monitor.Check{}
	if e := decodeClosed(n, &v); e != nil {
		return c, e
	}
	if v.PreferredIPProtocol == "" {
		v.PreferredIPProtocol = "ip4"
	}
	c.TCP = &monitor.TCPConfig{Port: v.Port, PreferredIPProtocol: v.PreferredIPProtocol, IPProtocolFallback: v.IPProtocolFallback}
	return c, nil
}
func decodeDNS(n yaml.Node) (monitor.Check, error) {
	var v dnsConfig
	c := monitor.Check{}
	if e := decodeClosed(n, &v); e != nil {
		return c, e
	}
	if v.RecordType == "" {
		v.RecordType = "A"
	}
	if v.Transport == "" {
		v.Transport = "udp"
	}
	if v.ExpectedRcodes == nil {
		v.ExpectedRcodes = []string{"NOERROR"}
	}
	sort.Strings(v.ExpectedRcodes)
	c.DNS = &monitor.DNSConfig{Resolver: v.Resolver, QueryName: v.QueryName, RecordType: v.RecordType, Transport: v.Transport, ExpectedRcodes: v.ExpectedRcodes, ExpectedRecordRegex: v.ExpectedRecordRegex}
	return c, nil
}
func decodeICMP(n yaml.Node) (monitor.Check, error) {
	var v icmpConfig
	c := monitor.Check{}
	if e := decodeClosed(n, &v); e != nil {
		return c, e
	}
	if v.PreferredIPProtocol == "" {
		v.PreferredIPProtocol = "ip4"
	}
	size := 32
	if v.PayloadSize != nil {
		size = *v.PayloadSize
	}
	c.ICMP = &monitor.ICMPConfig{PreferredIPProtocol: v.PreferredIPProtocol, IPProtocolFallback: v.IPProtocolFallback, PayloadSize: size, DontFragment: v.DontFragment}
	return c, nil
}
func ipProtocol(s string) bool { return s == "ip4" || s == "ip6" }

var dnsLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func dnsName(name string) bool {
	base := strings.TrimSuffix(name, ".")
	if len(base) == 0 || len(base) > 253 {
		return false
	}
	for _, l := range strings.Split(base, ".") {
		if !dnsLabel.MatchString(l) {
			return false
		}
	}
	return true
}
func endpoint(raw string) bool { a, e := netip.ParseAddrPort(raw); return e == nil && a.Port() != 0 }
func validateProbe(c monitor.Check) error {
	if len(c.Extract) > 0 && (c.Type != "http" || c.Runner != "native-http") {
		return fmt.Errorf("extraction requires native-http")
	}
	if e := validateExtract(c.Extract); e != nil {
		return e
	}
	if c.Type != "http" && c.Runner != "blackbox" {
		return fmt.Errorf("non-HTTP checks require blackbox")
	}
	switch c.Type {
	case "http":
		if len(c.BodyMatchesRegex) > 2048 {
			return fmt.Errorf("body regex too large")
		}
		if c.BodyMatchesRegex != "" {
			if _, e := regexp.Compile(c.BodyMatchesRegex); e != nil {
				return e
			}
		}
		if c.TCP != nil || c.DNS != nil || c.ICMP != nil {
			return fmt.Errorf("HTTP check has unrelated protocol fields")
		}
		u, e := url.Parse(c.URL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (c.Method != "GET" && c.Method != "HEAD") {
			return fmt.Errorf("invalid HTTP URL/method")
		}
		if c.MaxBodyBytes < 1 || c.MaxBodyBytes > 65536 || len(c.ExpectedStatus) == 0 || len(c.ExpectedStatus) > 500 {
			return fmt.Errorf("invalid HTTP limits")
		}
		for _, v := range c.ExpectedStatus {
			if v < 100 || v > 599 {
				return fmt.Errorf("invalid expected status")
			}
		}
	case "tcp":
		if c.TCP == nil || c.DNS != nil || c.ICMP != nil || c.TCP.Port < 1 || c.TCP.Port > 65535 || !ipProtocol(c.TCP.PreferredIPProtocol) {
			return fmt.Errorf("invalid TCP config")
		}
	case "icmp":
		if c.ICMP == nil || c.DNS != nil || c.TCP != nil || !ipProtocol(c.ICMP.PreferredIPProtocol) || c.ICMP.PayloadSize < 0 || c.ICMP.PayloadSize > 1400 || (c.ICMP.DontFragment && c.ICMP.PreferredIPProtocol != "ip4") {
			return fmt.Errorf("invalid ICMP config")
		}
	case "dns":
		if c.DNS == nil || c.TCP != nil || c.ICMP != nil {
			return fmt.Errorf("invalid DNS config")
		}
		d := c.DNS
		if !endpoint(d.Resolver) || !dnsName(d.QueryName) || (d.Transport != "udp" && d.Transport != "tcp") {
			return fmt.Errorf("invalid DNS resolver/query/transport")
		}
		types := map[string]bool{"A": true, "AAAA": true, "CNAME": true, "MX": true, "TXT": true, "NS": true}
		if !types[d.RecordType] {
			return fmt.Errorf("unsupported DNS record type")
		}
		rcodes := " NOERROR FORMERR SERVFAIL NXDOMAIN NOTIMP REFUSED YXDOMAIN YXRRSET NXRRSET NOTAUTH NOTZONE BADVERS BADSIG BADKEY BADTIME BADMODE BADNAME BADALG BADTRUNC BADCOOKIE "
		if len(d.ExpectedRcodes) == 0 || len(d.ExpectedRcodes) > 24 {
			return fmt.Errorf("invalid DNS rcodes")
		}
		for _, r := range d.ExpectedRcodes {
			if r == "" || strings.ContainsAny(r, " \t\r\n") || !strings.Contains(rcodes, " "+r+" ") {
				return fmt.Errorf("unsupported DNS rcode")
			}
		}
		if len(d.ExpectedRecordRegex) > 4096 {
			return fmt.Errorf("DNS regex too large")
		}
		if d.ExpectedRecordRegex != "" {
			if _, e := regexp.Compile(d.ExpectedRecordRegex); e != nil {
				return fmt.Errorf("invalid DNS answer regex: %w", e)
			}
		}
	default:
		return fmt.Errorf("unsupported check type")
	}
	if c.Type != "http" && (c.BodyMatchesRegex != "" || c.URL != "" || c.Method != "" || c.MaxBodyBytes != 0 || len(c.ExpectedStatus) != 0) {
		return fmt.Errorf("non-HTTP check has HTTP fields")
	}
	return nil
}
