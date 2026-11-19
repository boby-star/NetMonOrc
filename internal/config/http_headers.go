package config

import (
	"fmt"
	"net/http"
	"strings"
)

func normalizeHTTPHeaders(source map[string]string) (map[string]string, error) {
	if len(source) == 0 {
		return nil, nil
	}
	if len(source) > 32 {
		return nil, fmt.Errorf("at most 32 HTTP headers")
	}
	out := map[string]string{}
	size := 0
	for key, value := range source {
		if key == "" || strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("invalid HTTP header")
		}
		for _, r := range key {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
				return nil, fmt.Errorf("invalid HTTP header name")
			}
		}
		for _, r := range value {
			if (r < 32 && r != '\t') || r == 127 {
				return nil, fmt.Errorf("control character in HTTP header")
			}
		}
		name := http.CanonicalHeaderKey(key)
		lower := strings.ToLower(name)
		switch lower {
		case "authorization", "proxy-authorization", "cookie", "cookie2", "set-cookie", "host", "content-length", "connection", "transfer-encoding", "proxy-connection", "x-api-key", "x-auth-token":
			return nil, fmt.Errorf("credential or connection-control header forbidden")
		}
		if strings.HasSuffix(lower, "-token") || strings.HasSuffix(lower, "-secret") {
			return nil, fmt.Errorf("credential header forbidden")
		}
		if _, duplicate := out[name]; duplicate {
			return nil, fmt.Errorf("duplicate case-insensitive HTTP header")
		}
		size += len(name) + len(value)
		if size > 8192 {
			return nil, fmt.Errorf("HTTP headers exceed 8192 bytes")
		}
		out[name] = value
	}
	return out, nil
}
