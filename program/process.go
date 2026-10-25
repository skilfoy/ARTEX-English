package program

import (
	"net"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
)

// HardGate is process-wide. When it is on and no scope is loaded, tool input
// that names a remote host is refused. Local commands stay allowed.
var hardGate atomic.Bool

// SetHardGate turns the remote-call gate on or off.
func SetHardGate(on bool) { hardGate.Store(on) }

// HardGateOn reports whether a hard-gate skill is enforcing a scope file.
func HardGateOn() bool { return hardGate.Load() }

// MentionsRemote reports whether text names a host that is not loopback.
// File names such as range-scope.json are not hosts.
func MentionsRemote(text string) bool {
	for _, target := range extractTargets(text) {
		if remoteTarget(target) {
			return true
		}
	}
	return false
}

func remoteTarget(target string) bool {
	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err != nil || parsed.Host == "" {
			return true
		}
		return !localHost(parsed.Hostname())
	}
	host := target
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsUnspecified()
	}
	return !localHost(host)
}

func localHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	h = strings.TrimSuffix(h, ".")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// Hash is a stable digest of the loaded allow and deny sets. It is not a
// secret and it does not include flag values.
func (s *Scope) Hash() string {
	if s == nil {
		return ""
	}
	parts := []string{"habitat=" + s.Habitat}
	for _, rule := range s.Allow {
		if rule.Net != nil {
			parts = append(parts, "allow="+rule.Net.String())
		}
	}
	for _, rule := range s.Deny {
		if rule.Net != nil {
			parts = append(parts, "deny="+rule.Net.String())
		}
	}
	for name := range s.Names {
		parts = append(parts, "name="+name)
	}
	for name := range s.Denied {
		parts = append(parts, "denied="+name)
	}
	sort.Strings(parts)
	return HashText(strings.Join(parts, "\n"))
}

// AdvisoryRoles are the specialist seats. Seating them does not block writes
// unless the desk is in strict mode.
var AdvisoryRoles = []string{"recon", "identity", "web", "evidence", "retest"}

type manifestSet struct {
	byName map[string]MCPServer
}

var processManifest atomic.Pointer[manifestSet]
var processMount atomic.Pointer[CodeGraph]
var processMountRoot atomic.Pointer[string]

// InstallManifest publishes the MCP allowlist. Nil clears it, which restores
// the historical pass-through for tool names.
func InstallManifest(servers []MCPServer) {
	if len(servers) == 0 {
		processManifest.Store(nil)
		return
	}
	set := &manifestSet{byName: map[string]MCPServer{}}
	for _, server := range servers {
		set.byName[server.Name] = server
	}
	processManifest.Store(set)
}

// ManifestAllows reports whether a tool name may run. Names that are not
// mcp__server__tool are ignored. When no manifest is installed, every name
// is allowed.
func ManifestAllows(tool string) (bool, string) {
	set := processManifest.Load()
	if set == nil || !strings.HasPrefix(tool, "mcp__") {
		return true, ""
	}
	parts := strings.Split(tool, "__")
	if len(parts) < 3 || parts[1] == "" {
		return false, "mcp tool name is not server__tool"
	}
	if _, ok := set.byName[parts[1]]; !ok {
		return false, "mcp server is not in the manifest"
	}
	return true, ""
}

// InstallMountGraph publishes a read-only code graph. Nil clears it.
// The graph is file paths and import edges, not file contents.
func InstallMountGraph(graph *CodeGraph) { processMount.Store(graph) }

// MountGraph returns the installed code graph, or nil.
func MountGraph() *CodeGraph { return processMount.Load() }

// InstallMountRoot records the tree the graph was built from. Empty clears it.
func InstallMountRoot(root string) {
	if root == "" {
		processMountRoot.Store(nil)
		return
	}
	processMountRoot.Store(&root)
}

// MountRoot returns the installed mount, or an empty string.
func MountRoot() string {
	root := processMountRoot.Load()
	if root == nil {
		return ""
	}
	return *root
}
