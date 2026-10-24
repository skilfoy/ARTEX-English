package program

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	"golang.org/x/net/idna"
)

// Scope is a fail-closed authorization edge. Deny wins. An empty allow list
// allows nothing. Loading a file that allows the whole internet is an error.
type Scope struct {
	Habitat string
	Allow   []netRule
	Deny    []netRule
	Names   map[string]nameRec
	Denied  map[string]nameRec
}

type netRule struct {
	Net    *net.IPNet
	Reason string
}

type nameRec struct {
	FQDN string
	IP   net.IP
	Zone string
}

type scopeFile struct {
	Version     int        `json:"version"`
	Habitat     string     `json:"habitat"`
	Default     string     `json:"default"`
	Allow       []cidrRule `json:"allow"`
	Deny        []cidrRule `json:"deny"`
	Names       []nameRule `json:"names"`
	DeniedNames []nameRule `json:"deniedNames"`
	StartingURL string     `json:"startingUrl"`
}

type cidrRule struct {
	CIDR   string `json:"cidr"`
	Zone   string `json:"zone"`
	Reason string `json:"reason"`
}

type nameRule struct {
	FQDN string `json:"fqdn"`
	IP   string `json:"ip"`
	Zone string `json:"zone"`
}

// Decision is the result of checking one target or one tool input.
type Decision struct {
	Allowed bool
	Reason  string
	Target  string
}

var processScope atomic.Pointer[Scope]

// InstallProcessScope publishes the scope every new guard should enforce.
// Nil clears it. Existing guards keep the pointer they were given.
func InstallProcessScope(s *Scope) { processScope.Store(s) }

// ProcessScope returns the scope installed for the process, or nil.
func ProcessScope() *Scope { return processScope.Load() }

// InstallFromEnv loads ARTEX_SCOPE_FILE when it is set and installs it.
// An empty variable is not an error: the guard stays in its historical
// pass-through mode until a scope is attached.
func InstallFromEnv() error {
	path := strings.TrimSpace(os.Getenv("ARTEX_SCOPE_FILE"))
	if path == "" {
		return nil
	}
	s, err := LoadFile(path)
	if err != nil {
		return err
	}
	InstallProcessScope(s)
	return nil
}

// LoadFile reads a scope file from disk.
func LoadFile(path string) (*Scope, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse builds a scope from a range-scope document. The default, when set,
// must be deny. 0.0.0.0/0 and ::/0 are refused.
func Parse(raw []byte) (*Scope, error) {
	var file scopeFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("scope: %w", err)
	}
	if file.Default != "" && file.Default != "deny" {
		return nil, fmt.Errorf("scope: default must be deny, got %q", file.Default)
	}
	s := &Scope{
		Habitat: file.Habitat,
		Names:   map[string]nameRec{},
		Denied:  map[string]nameRec{},
	}
	var err error
	if s.Allow, err = parseNets(file.Allow, false); err != nil {
		return nil, err
	}
	if s.Deny, err = parseNets(file.Deny, true); err != nil {
		return nil, err
	}
	for _, rule := range file.Names {
		rec, err := parseName(rule)
		if err != nil {
			return nil, err
		}
		s.Names[rec.FQDN] = rec
	}
	for _, rule := range file.DeniedNames {
		rec, err := parseName(rule)
		if err != nil {
			return nil, err
		}
		s.Denied[rec.FQDN] = rec
	}
	if file.StartingURL != "" {
		if d := s.Check(file.StartingURL); !d.Allowed {
			return nil, fmt.Errorf("scope: starting url is not allowed: %s", d.Reason)
		}
	}
	return s, nil
}

func parseNets(rules []cidrRule, deny bool) ([]netRule, error) {
	out := make([]netRule, 0, len(rules))
	for _, rule := range rules {
		cidr := strings.TrimSpace(rule.CIDR)
		if cidr == "" {
			return nil, fmt.Errorf("scope: empty cidr")
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("scope: cidr %q: %w", cidr, err)
		}
		ones, bits := network.Mask.Size()
		if ones == 0 && (bits == 32 || bits == 128) {
			return nil, fmt.Errorf("scope: %s is the whole internet and is refused", cidr)
		}
		reason := strings.TrimSpace(rule.Reason)
		if reason == "" {
			reason = strings.TrimSpace(rule.Zone)
		}
		if reason == "" && deny {
			reason = "denied"
		}
		out = append(out, netRule{Net: network, Reason: reason})
	}
	return out, nil
}

func parseName(rule nameRule) (nameRec, error) {
	fqdn, err := canonicalHost(rule.FQDN)
	if err != nil || fqdn == "" || net.ParseIP(fqdn) != nil {
		return nameRec{}, fmt.Errorf("scope: bad name %q", rule.FQDN)
	}
	rec := nameRec{FQDN: fqdn, Zone: rule.Zone}
	if strings.TrimSpace(rule.IP) != "" {
		ip := net.ParseIP(strings.TrimSpace(rule.IP))
		if ip == nil {
			return nameRec{}, fmt.Errorf("scope: bad address for %s", fqdn)
		}
		rec.IP = ip
	}
	return rec, nil
}

func canonicalHost(host string) (string, error) {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return "", nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", err
	}
	return ascii, nil
}

// Check reads tool input and allows it only when every target it names is in
// scope. Input that names no target is allowed: it is not a network action.
// The first out-of-scope target denies the whole input.
func (s *Scope) Check(text string) Decision {
	if s == nil {
		return Decision{Allowed: false, Reason: "no scope loaded"}
	}
	targets := extractTargets(text)
	if len(targets) == 0 {
		return Decision{Allowed: true, Reason: "no target"}
	}
	var seenAllow bool
	for _, target := range targets {
		d := s.Decide(target)
		if !d.Allowed {
			return d
		}
		seenAllow = true
	}
	if !seenAllow {
		return Decision{Allowed: false, Reason: "no target was in scope"}
	}
	return Decision{Allowed: true, Reason: "in scope"}
}

// Decide classifies one host, address, CIDR, or URL.
func (s *Scope) Decide(target string) Decision {
	if s == nil {
		return Decision{Allowed: false, Target: target, Reason: "no scope loaded"}
	}
	raw := strings.TrimSpace(target)
	if raw == "" {
		return Decision{Allowed: false, Reason: "empty target"}
	}
	if strings.Contains(raw, "://") || strings.Contains(raw, "@") && !strings.Contains(raw, " ") {
		return s.decideURL(raw)
	}
	if strings.Contains(raw, "/") {
		return s.decideCIDR(raw)
	}
	if ip := net.ParseIP(raw); ip != nil {
		return s.decideIP(ip, raw)
	}
	host, err := canonicalHost(raw)
	if err != nil || host == "" {
		return Decision{Allowed: false, Target: raw, Reason: "unreadable host"}
	}
	return s.decideHost(host)
}

func (s *Scope) decideURL(raw string) Decision {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		if strings.Contains(raw, "@") {
			return Decision{Allowed: false, Target: raw, Reason: "credentials in url"}
		}
		return Decision{Allowed: false, Target: raw, Reason: "unreadable url"}
	}
	if parsed.User != nil {
		return Decision{Allowed: false, Target: raw, Reason: "credentials in url"}
	}
	host := parsed.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return s.decideIP(ip, host)
	}
	name, err := canonicalHost(host)
	if err != nil || name == "" {
		return Decision{Allowed: false, Target: raw, Reason: "unreadable host"}
	}
	return s.decideHost(name)
}

func (s *Scope) decideCIDR(raw string) Decision {
	_, network, err := net.ParseCIDR(raw)
	if err != nil {
		return Decision{Allowed: false, Target: raw, Reason: "unreadable network"}
	}
	for _, rule := range s.Deny {
		if overlaps(network, rule.Net) {
			return Decision{Allowed: false, Target: raw, Reason: reasonOr(rule.Reason, "denied network")}
		}
	}
	for _, rule := range s.Allow {
		if subset(network, rule.Net) {
			return Decision{Allowed: true, Target: raw, Reason: "in scope"}
		}
	}
	return Decision{Allowed: false, Target: raw, Reason: "not inside an allowed network"}
}

func (s *Scope) decideIP(ip net.IP, shown string) Decision {
	if isAlwaysDeniedIP(ip) {
		return Decision{Allowed: false, Target: shown, Reason: "address is never a target"}
	}
	for _, rule := range s.Deny {
		if rule.Net.Contains(ip) {
			return Decision{Allowed: false, Target: shown, Reason: reasonOr(rule.Reason, "denied network")}
		}
	}
	for _, rule := range s.Allow {
		if rule.Net.Contains(ip) {
			return Decision{Allowed: true, Target: shown, Reason: "in scope"}
		}
	}
	return Decision{Allowed: false, Target: shown, Reason: "not inside an allowed network"}
}

func (s *Scope) decideHost(host string) Decision {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return Decision{Allowed: false, Target: host, Reason: "address is never a target"}
	}
	if rec, ok := s.Denied[host]; ok {
		reason := "name is denied"
		if rec.Zone != "" {
			reason = "name is denied (" + rec.Zone + ")"
		}
		return Decision{Allowed: false, Target: host, Reason: reason}
	}
	rec, ok := s.Names[host]
	if !ok {
		return Decision{Allowed: false, Target: host, Reason: "name is not in scope"}
	}
	if rec.IP != nil {
		d := s.decideIP(rec.IP, host)
		if !d.Allowed {
			return d
		}
	}
	return Decision{Allowed: true, Target: host, Reason: "in scope"}
}

func reasonOr(reason, fallback string) string {
	if strings.TrimSpace(reason) == "" {
		return fallback
	}
	return reason
}

func isAlwaysDeniedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 169 && v4[1] == 254 && v4[2] == 169 && v4[3] == 254 {
		return true
	}
	return false
}

func overlaps(a, b *net.IPNet) bool {
	if !sameFamily(a.IP, b.IP) {
		return false
	}
	return a.Contains(b.IP) || b.Contains(a.IP)
}

func subset(inner, outer *net.IPNet) bool {
	if !sameFamily(inner.IP, outer.IP) {
		return false
	}
	if !outer.Contains(inner.IP) || !outer.Contains(lastIP(inner)) {
		return false
	}
	return true
}

func sameFamily(a, b net.IP) bool {
	return (a.To4() != nil) == (b.To4() != nil)
}

func lastIP(network *net.IPNet) net.IP {
	ip := network.IP
	mask := network.Mask
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		if len(mask) == net.IPv6len {
			mask = mask[12:]
		}
	}
	if len(mask) != len(ip) {
		return network.IP
	}
	out := make(net.IP, len(ip))
	for i := range ip {
		out[i] = ip[i] | ^mask[i]
	}
	return out
}

var (
	reURL  = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s'"<>]+`)
	reIPv4 = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)(?:/(?:3[0-2]|[12]?\d))?\b`)
	reHost = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}\b`)
)

var fileSuffix = map[string]bool{
	"json": true, "js": true, "mjs": true, "cjs": true, "go": true, "py": true,
	"md": true, "txt": true, "yml": true, "yaml": true, "toml": true, "lock": true,
	"mod": true, "sum": true, "png": true, "svg": true, "css": true, "html": true,
	"xml": true, "sh": true, "tf": true, "sql": true, "log": true,
}

func extractTargets(text string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(v string) {
		v = strings.TrimRight(v, ".,);]")
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, match := range reURL.FindAllString(text, -1) {
		add(match)
	}
	for _, match := range reIPv4.FindAllString(text, -1) {
		add(match)
	}
	for _, match := range reHost.FindAllString(text, -1) {
		if isFileName(match) {
			continue
		}
		add(match)
	}
	return out
}

func isFileName(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return false
	}
	return fileSuffix[strings.ToLower(parts[len(parts)-1])]
}
