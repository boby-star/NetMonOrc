package config

import (
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"gopkg.in/yaml.v3"
	"regexp"
)

type extractSpec struct {
	Name      string    `yaml:"name"`
	From      string    `yaml:"from"`
	Using     string    `yaml:"using"`
	Pattern   string    `yaml:"pattern"`
	Group     yaml.Node `yaml:"group"`
	As        string    `yaml:"as"`
	OnMissing string    `yaml:"onMissing"`
}

func compileExtract(spec []extractSpec) ([]monitor.Extractor, error) {
	if len(spec) > 16 {
		return nil, fmt.Errorf("at most 16 extractors supported per check")
	}
	var out []monitor.Extractor
	for _, s := range spec {
		if s.From != "response.body" || (s.Using != "" && s.Using != "regex") {
			return nil, fmt.Errorf("only response.body regex extraction supported")
		}
		r, e := regexp.Compile(s.Pattern)
		if e != nil {
			return nil, e
		}
		group := 0
		switch s.Group.Tag {
		case "!!int":
			if e = s.Group.Decode(&group); e != nil {
				return nil, e
			}
		case "!!str":
			group = r.SubexpIndex(s.Group.Value)
		default:
			return nil, fmt.Errorf("capture group must be a name or positive integer")
		}
		as := s.As
		if as == "" {
			as = "string"
		}
		missing := s.OnMissing
		if missing == "" {
			missing = "unknown"
		}
		out = append(out, monitor.Extractor{Name: s.Name, Pattern: s.Pattern, Group: group, As: as, OnMissing: missing})
	}
	return out, validateExtract(out)
}
func validateExtract(out []monitor.Extractor) error {
	if len(out) > 16 {
		return fmt.Errorf("too many extractors")
	}
	seen := map[string]bool{}
	for _, x := range out {
		if !idPattern.MatchString(x.Name) || seen[x.Name] || len(x.Pattern) == 0 || len(x.Pattern) > 2048 {
			return fmt.Errorf("invalid extractor name/pattern")
		}
		seen[x.Name] = true
		r, e := regexp.Compile(x.Pattern)
		if e != nil {
			return e
		}
		if x.Group < 1 || x.Group > r.NumSubexp() {
			return fmt.Errorf("capture group does not exist")
		}
		if x.As != "string" && x.As != "integer" && x.As != "float" && x.As != "boolean" {
			return fmt.Errorf("invalid extracted type")
		}
		if x.OnMissing != "unknown" && x.OnMissing != "error" {
			return fmt.Errorf("invalid onMissing")
		}
	}
	return nil
}
