package agent

import (
	"context"
	"encoding/json"

	actool "github.com/Autumn-27/norma/tool"
)

// This document is for[Internal Tools]From pure code to enumerable. DB Overrided directories:
//   - BuiltinToolSeeds():Three to execute. agent Expand the integration of built-in tools seed Record(key +
//     Description + Parameter schema + Default bound agent),It's for the service end. tools Table.
//   - ToolResolve Hook: Press on running DB inside tools Line to assembled tools[Press agent Filter +
//     Overwrite Description/schema + Default for input parameters].key/handler Still on the code level.,DB Change only[Text and Defaults].
// handler(Call ) Always from the code.——DB We can't change it. We can change the description and the defaults..

// ToolSeed It's a broadcastable snapshot of a built-in tool.:key i.e. CoreTool.Name()(With handler Tie her to death.,
// UI Read only),Desc/Schema Tool definition from code,Agents It's the code that's given it by default. agent.
type ToolSeed struct {
	Key    string         // = CoreTool.Name(),Primary key, unchangeable
	Desc   string         // Top Description UI override)
	Schema map[string]any // Parameter JSON-Schema(Structure read-only,description/default Available at UI Change)
	Agents []string       // Default bound agent key(worker/planner/mainagent)
}

// builtinToolsByAgent With one.[Empty shell only]ToolSet(nil stores)Construct each execution agent of
// Area tool set. Tool Construct function only closes Spec,Construct period does not solve references store,So nil Clear.——
// These tools are only for reading here. Name()/Description()/InputSchema(),Never. Call.
//
// I mean it.[Not included]SDK General Tools actool.DefaultTools()(Read/Write/Edit/MultiEdit/LS/Glob/
// Grep/Bash):Each of them. agent All fixed, no.[To whom?]The trade-offs, most of them. Prompt()
// Lee (this table only covers) Description(),It's a half-covered misdirection. Oh, no. seed → None DB row → ToolResolve
// As it was, it was not covered and the behaviour was consistent with that of the past. Only artex Portfolio of your own field tools.
func builtinToolsByAgent() map[string][]actool.CoreTool {
	ts := NewToolSet(nil, "")
	return map[string][]actool.CoreTool{
		"mainagent": ts.MainAgentTools(),
		"planner":   ts.PlannerTools(),
		"worker":    ts.WorkerTools(),
		// goals(Target Dismantling) Default binding set_goals + set_constraints:They're the ones who took the target.,
		// The extracting operational constraints are written into the library. and mainagent Sharing the same tools.,web End recapable/schema,Press agent Check.
		"goals": {ts.setGoals(), ts.setConstraints()},
		// auto Default breach report + Asset management tool, other domain tools UI Check on demand.
		// The new library. seed writing;old seedAutoDefaultBindings Migration.
		"auto": {ts.addFinding(), ts.insertAssets(), ts.addCompanyScope(), ts.listAssets(), ts.listCompanies()},
		// pentest(Independent infiltration agent)Default binding: checking assets / Assets inserted / Reporting loopholes / Check out the holes. / Business..
		// The new library. seed writing;old seedPentestDefaultBindings Migration.
		"pentest": {ts.listAssets(), ts.insertAssets(), ts.addFinding(), ts.listFindings(), ts.listCompanies()},
	}
}

// defaultUnbound:These. system Tools will be entered as usual.(web Port visible, manual press agent Check) but
// Default[Nothing. agent]——ToolResolve All empty bound tools agent Discard them all.,Remarkable opt-in.
// It's still there. agent of base In the tool set(As goal_met at PlannerTools):- Jean. seed Yes.
// Construct it to get it. desc/schema,Second is when the user is manually tied back and running. base It's in there.,ToolResolve I can keep it..
//
// goal_met:Round and round. prove_goal,Directly from the global scene.[The whole mission is complete.],Powerful and at risk of error.,Also with
// [prove_goal Mark last target → Autotaker]Repeat,So, by default, no. agent,We'll tie it manually when we need it..
var defaultUnbound = map[string]bool{"goal_met": true}

// BuiltinToolSeeds Let's split up. agent Internal toolsets to merge into seed List: Tools with the same name list_assets
// Multiple agent We've got one.,Agents Summon;defaultUnbound The tools in it are forced to be empty..
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
			agents = []string{} // Enter the directory, bind manually, but not by default agent(Save [] instead of null,Consistent with other tools)
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
// Default Participation get injected. Tools with no matching DB row (MCP/skill/host tools like
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
// null. Structure (names/types/required) is untouched — onlyDefault value are merged in.
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
