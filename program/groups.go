package program

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/idna"
)

// HostGroup is a named set of hosts. Intersection never adds a host that
// was not already in every group.
type HostGroup struct {
	Name  string
	Hosts []string
}

// CleanHost returns a lowercase host. Credentials, guardrail addresses, and
// empty names are refused. A URL contributes its hostname only.
func CleanHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("group: empty host")
	}
	if strings.Contains(raw, "@") {
		return "", fmt.Errorf("group: credentials in host")
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return "", fmt.Errorf("group: unreadable host")
		}
		raw = parsed.Hostname()
	}
	raw = strings.Trim(strings.TrimSuffix(raw, "."), "[]")
	raw = strings.ToLower(raw)
	if i := strings.Index(raw, "/"); i >= 0 {
		raw = raw[:i]
	}
	if GuardrailHost(raw) {
		return "", fmt.Errorf("group: guardrail host")
	}
	ascii, err := idna.Lookup.ToASCII(raw)
	if err != nil || ascii == "" || strings.Contains(ascii, " ") {
		return "", fmt.Errorf("group: bad host")
	}
	return ascii, nil
}

// GuardrailHost is an address that is never a target: loopback, link-local,
// the unspecified address, and the cloud metadata address.
func GuardrailHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "localhost" || h == "metadata.google.internal" {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return ip.Equal(net.ParseIP("169.254.169.254"))
}

// IntersectGroups returns hosts present in every group, cleaned. An empty
// intersection is not an error.
func IntersectGroups(groups []HostGroup) ([]string, error) {
	if len(groups) == 0 {
		return nil, fmt.Errorf("group: no groups")
	}
	sets := make([]map[string]bool, 0, len(groups))
	for _, group := range groups {
		set := map[string]bool{}
		for _, host := range group.Hosts {
			cleaned, err := CleanHost(host)
			if err != nil {
				return nil, fmt.Errorf("group %s: %w", group.Name, err)
			}
			set[cleaned] = true
		}
		sets = append(sets, set)
	}
	var out []string
	for host := range sets[0] {
		keep := true
		for _, set := range sets[1:] {
			if !set[host] {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, host)
		}
	}
	sort.Strings(out)
	return out, nil
}
