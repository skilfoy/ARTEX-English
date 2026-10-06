package agent

import (
	"context"
	"encoding/json"

	actool "github.com/Autumn-27/norma/tool"
)

// This file turns built-in tools from pure code into a catalog the DB can override:
//   - BuiltinToolSeeds() expands the three execution agents' built-in tool sets into
//     seed records (key + description + parameter schema + default bound agents) for
//     the server to idempotently seed into the tools table on startup.
//   - The ToolResolve hook, at runtime, filters already-assembled tools by the tools
//     rows in the DB (filter by agent, override description/schema, and inject
//     parameter defaults). key and handler stay in code; the DB only changes the
//     prose and the defaults.
// The handler (Call behavior) always comes from code — the DB cannot change it, only
// the description the model sees and the default arguments.

// ToolSeed is a seedable snapshot of one built-in tool. key is CoreTool.Name()
// (bound to the handler; the UI is read-only). Desc and Schema come from the tool
// definition in code. Agents is which agents the code binds it to by default.
type ToolSeed struct {
	Key    string         // = CoreTool.Name(), primary key, immutable
	Desc   string         // top-level description (overridable in the UI)
	Schema map[string]any // parameter JSON Schema (structure read-only; description/default editable in the UI)
	Agents []string       // default bound agent keys (worker/planner/mainagent)
}

// builtinToolsByAgent builds each execution agent's domain tool set with a
// read-only empty-shell ToolSet (nil stores). Constructors only close over the
// Spec and do not dereference the store, so nil is safe — these tools are used
// here only to read Name()/Description()/InputSchema(), never Call.
//
// SDK general tools actool.DefaultTools() (Read/Write/Edit/MultiEdit/LS/Glob/
// Grep/Bash) are deliberately excluded. Every agent always has them, there is no
// "bound to whom" choice, and most of their text lives in Prompt() (this table
// only covers Description(), so seeding them would be a misleading half-override).
// Not seeded means no DB row, so ToolResolve passes them through unchanged and
// behavior matches the past. Only ARTEX's own domain tools are catalogued.
func builtinToolsByAgent() map[string][]actool.CoreTool {
	ts := NewToolSet(nil, "")
	return map[string][]actool.CoreTool{
		"mainagent": ts.MainAgentTools(),
		"planner":   ts.PlannerTools(),
		"worker":    ts.WorkerTools(),
		// goals (the goal decomposer) defaults to set_goals + set_constraints: those
		// write the decomposed goals and the extracted operation constraints into the
		// store. They share the same managed tools as mainagent; the web UI can edit
		// description/schema and toggle them per agent.
		"goals": {ts.setGoals(), ts.setConstraints()},
		// auto defaults to finding-report and asset-management tools; other domain
		// tools can be checked in the UI as needed. New databases are written by this
		// seed; old databases are migrated by seedAutoDefaultBindings.
		"auto": {ts.addFinding(), ts.insertAssets(), ts.addCompanyScope(), ts.listAssets(), ts.listCompanies()},
		// pentest (the standalone assessment agent) defaults to: list assets / insert
		// assets / report findings / list findings / list companies. New databases are
		// written by this seed; old databases are migrated by seedPentestDefaultBindings.
		"pentest": {ts.listAssets(), ts.insertAssets(), ts.addFinding(), ts.listFindings(), ts.listCompanies()},
	}
}

// defaultUnbound: these system tools are still catalogued (visible on the web, and
// can be checked per agent by hand) but are bound to no agent by default.
// ToolResolve drops an unbound tool for every agent, so it is explicit opt-in.
// They stay in some agent's base tool set (goal_met is in PlannerTools) for two
// reasons: the seed can construct them to obtain desc/schema, and after a user
// binds them back the runtime base still contains them so ToolResolve can keep them.
//
// goal_met skips proving goals one by one and declares the whole task complete from
// the global picture. It is powerful and easy to misuse, and it overlaps with
// "prove_goal marks the last goal, then the task completes automatically", so no
// agent gets it by default. Bind it by hand when needed.
var defaultUnbound = map[string]bool{"goal_met": true}

// BuiltinToolSeeds deduplicates each agent's built-in tools into a seed list.
// Tools with the same name (list_assets on several agents) become one row, and
// Agents is the union. Tools in defaultUnbound are forced to an empty binding.
func BuiltinToolSeeds() []ToolSeed {
	byAgent := builtinToolsByAgent()
	order := []string{"mainagent", "goals", "planner", "worker", "auto", "pentest"}

	type acc struct {
		tool   actool.CoreTool
		agents []string
	}
	m := map[string]*acc{}
	var keys []string
	for _, ak := range order {
		for _, t := range byAgent[ak] {
			a, ok := m[t.Name()]
			if !ok {
				a = &acc{tool: t}
				m[t.Name()] = a
				keys = append(keys, t.Name())
			}
			a.agents = append(a.agents, ak)
		}
	}

	out := make([]ToolSeed, 0, len(keys))
	for _, k := range keys {
		a := m[k]
		agents := a.agents
		if defaultUnbound[k] {
			agents = []string{} // in the catalog and manually bindable, but bound to no agent by default (store [] not null, consistent with the other tools)
		}
		out = append(out, ToolSeed{
			Key:    k,
			Desc:   a.tool.Description(),
			Schema: a.tool.InputSchema(),
			Agents: agents,
		})
	}
	return out
}

// ToolResolve, if set, post-processes an agent's fully-assembled tool list against
// the DB tools table: it drops tools not bound to this agent (or globally disabled)
// and wraps the rest so the model sees the DB-overridden description/schema and
// Default arguments get injected. Tools with no matching DB row (MCP/skill/host tools like
// traffic) pass through untouched. nil = tools unchanged. Wired in server/assembly.go.
var ToolResolve func(ctx context.Context, agentKey string, tools []actool.CoreTool) []actool.CoreTool

// DecorateTool wraps t so Description()/InputSchema() report the DB overrides and
// Call() injects scalar parameter defaults (from schema's "default" props) whenever
// the model omitted them. Name/Prompt/permission/scheduler flags delegate to t, so
// the tool's identity and handler are unchanged. Empty desc/schema fall back to t's.
func DecorateTool(t actool.CoreTool, desc string, schema map[string]any) actool.CoreTool {
	if desc == "" {
		desc = t.Description()
	}
	if len(schema) == 0 {
		schema = t.InputSchema()
	}
	return &overriddenTool{CoreTool: t, desc: desc, schema: schema}
}

// overriddenTool is a CoreTool decorator: it embeds the original (so all behavioral
// methods — Prompt/IsReadOnly/IsConcurrencySafe/CheckPermissions/Name — delegate)
// and overrides only the model-facing description/schema plus default injection.
type overriddenTool struct {
	actool.CoreTool
	desc   string
	schema map[string]any
}

func (o *overriddenTool) Description() string         { return o.desc }
func (o *overriddenTool) InputSchema() map[string]any { return o.schema }

func (o *overriddenTool) Call(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
	return o.CoreTool.Call(ctx, injectDefaults(in, o.schema), tc)
}

// injectDefaults fills scalar parameter defaults declared in the (possibly edited)
// schema into the input JSON whenever the model omitted the field or left it empty/
// null. Structure (names/types/required) is untouched — only default values are merged in.
func injectDefaults(in json.RawMessage, schema map[string]any) json.RawMessage {
	defs := scalarDefaults(schema)
	if len(defs) == 0 {
		return in
	}
	m := map[string]json.RawMessage{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &m); err != nil {
			return in // non-object input: don't touch it
		}
	}
	changed := false
	for k, dv := range defs {
		if cur, ok := m[k]; !ok || isEmptyJSON(cur) {
			m[k] = dv
			changed = true
		}
	}
	if !changed {
		return in
	}
	b, err := json.Marshal(m)
	if err != nil {
		return in
	}
	return b
}

// scalarDefaults extracts properties[k]["default"] for scalar params (string/
// integer/number/boolean). Array/object defaults are skipped: merging them is
// ambiguous and not worth the surprise.
func scalarDefaults(schema map[string]any) map[string]json.RawMessage {
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	out := map[string]json.RawMessage{}
	for name, raw := range props {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		dv, ok := p["default"]
		if !ok || dv == nil {
			continue
		}
		switch p["type"] {
		case "string", "integer", "number", "boolean":
			if b, err := json.Marshal(dv); err == nil {
				out[name] = b
			}
		}
	}
	return out
}

func isEmptyJSON(raw json.RawMessage) bool {
	s := string(raw)
	return s == "null" || s == `""`
}
