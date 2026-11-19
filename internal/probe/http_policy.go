package probe

import (
	"context"
	"errors"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

var errHTTPPolicy = errors.New("HTTP target policy denied")

// Hostname admission is provisional: every actual socket is pinned to a
// policy-checked IP after one Agent-side lookup. No second resolver gets the name.
func allowedHTTPCheck(c monitor.Check, allowed []netip.Prefix) error {
	u, err := url.Parse(c.URL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.Hostname() == "" {
		return fmt.Errorf("%w: invalid URL", errHTTPPolicy)
	}
	if len(allowed) == 0 {
		return fmt.Errorf("%w: no allowed CIDRs", errHTTPPolicy)
	}
	if c.ConnectIP != "" {
		ip, err := netip.ParseAddr(c.ConnectIP)
		if err != nil {
			return fmt.Errorf("%w: invalid connectIP", errHTTPPolicy)
		}
		if err = allowedAddress(ip, allowed); err != nil {
			return fmt.Errorf("%w: %v", errHTTPPolicy, err)
		}
		return nil
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
		if err = allowedAddress(ip, allowed); err != nil {
			return fmt.Errorf("%w: %v", errHTTPPolicy, err)
		}
	}
	return nil
}
func policyHTTPDial(c monitor.Check, allowed []netip.Prefix) func(context.Context, string, string) (net.Conn, error) {
	origin, _ := url.Parse(c.URL)
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		var ips []netip.Addr
		if c.ConnectIP != "" && strings.EqualFold(host, origin.Hostname()) {
			ip, err := netip.ParseAddr(c.ConnectIP)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid connectIP", errHTTPPolicy)
			}
			ips = []netip.Addr{ip}
		} else if ip, err := netip.ParseAddr(host); err == nil {
			ips = []netip.Addr{ip}
		} else {
			ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
		}
		sort.Slice(ips, func(i, j int) bool {
			a, b := ips[i].Unmap(), ips[j].Unmap()
			if a.Is4() != b.Is4() {
				return a.Is4()
			}
			return a.Less(b)
		})
		var last error
		permitted := false
		for _, ip := range ips {
			if allowedAddress(ip, allowed) != nil {
				continue
			}
			permitted = true
			conn, err := (&net.Dialer{Timeout: c.Timeout, KeepAlive: -time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		if !permitted {
			return nil, fmt.Errorf("%w: no allowed resolved IP", errHTTPPolicy)
		}
		return nil, last
	}
}
