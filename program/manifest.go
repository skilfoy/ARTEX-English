package program

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// MCPServer is a remote tool server the process may call. AuthRef is the
// name of an environment variable, never the secret itself.
type MCPServer struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Phases  []string `json:"phases,omitempty"`
	AuthRef string   `json:"authRef,omitempty"`
}

var authRefPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

var knownPhases = map[string]bool{
	"recon": true, "web": true, "identity": true,
	"evidence": true, "retest": true, "report": true,
}

// ParseManifest validates a JSON list of MCP servers. The returned list is
// safe to log: query strings and userinfo are stripped.
func ParseManifest(raw []byte) ([]MCPServer, error) {
	var servers []MCPServer
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	seen := map[string]bool{}
	out := make([]MCPServer, 0, len(servers))
	for i, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name == "" || strings.ContainsAny(name, " \t\r\n") {
			return nil, fmt.Errorf("manifest: server %d has an empty name", i)
		}
		if seen[name] {
			return nil, fmt.Errorf("manifest: duplicate server %s", name)
		}
		seen[name] = true
		cleaned, err := cleanServerURL(server.URL)
		if err != nil {
			return nil, fmt.Errorf("manifest: %s: %w", name, err)
		}
		auth := strings.TrimSpace(server.AuthRef)
		if auth != "" && !authRefPattern.MatchString(auth) {
			return nil, fmt.Errorf("manifest: %s authRef must be an environment variable name", name)
		}
		phases, err := cleanPhases(server.Phases)
		if err != nil {
			return nil, fmt.Errorf("manifest: %s: %w", name, err)
		}
		out = append(out, MCPServer{Name: name, URL: cleaned, Phases: phases, AuthRef: auth})
	}
	return out, nil
}

func cleanServerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("unreadable url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("url scheme must be http or https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("url must not carry credentials")
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func cleanPhases(phases []string) ([]string, error) {
	if len(phases) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(phases))
	for _, phase := range phases {
		phase = strings.ToLower(strings.TrimSpace(phase))
		if !knownPhases[phase] {
			return nil, fmt.Errorf("unknown phase %q", phase)
		}
		if seen[phase] {
			continue
		}
		seen[phase] = true
		out = append(out, phase)
	}
	sort.Strings(out)
	return out, nil
}
