package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	acperm "github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
	"github.com/skilfoy/ARTEX-English/db"
)

// goalsDefaultTmpl is the built-in EDITABLE body (section [A]) of the goals-decomposer
// prompt, seeded into agent_prompts. No template vars are used today.
const goalsDefaultTmpl = `You decompose the operator's task into independently verifiable final outcomes. Identify the result the operator wants, rather than reconnaissance, test steps, or exploitation methods. Respond in English.

First, extract explicit operational constraints from the task goal and description. Register each with set_constraints as deny for prohibited operations or allow for a stated limitation or permission. Make every constraint self-contained by replacing references such as "this target" with a specific target already given in the task. Record only constraints supported by the operator's words. If the type is uncertain, choose deny. Omit this step if no operational constraint is stated.

Next, use set_goals. Use one goal for a single final outcome. Split genuinely independent final outcomes into separate goals. Include vulnclass only when a clear vulnerability class applies. Do not invent goals or treat reconnaissance, vulnerability analysis, exploitation steps, or verification steps as final outcomes.`

// goalsScopeTail is the code-owned tail appended after the editable goals body
// WHEN an asset store + task context are available. It teaches the decomposer to
// also lift the explicit asset scope out of the goal/description and register it
// via add_task_scope. Kept in code (not the DB-editable body) so it always applies
// on released DBs and can't be edited away — same pattern as the trafficTool tail.
const goalsScopeTail = `

Also register the asset scope explicitly stated in the task goal or description with add_task_scope. This scope is the authorization boundary and the denominator for asset coverage. Use the narrowest scope the operator named. Respond in English.

For a URL or hostname with subdomains, register the full hostname as kind=subdomain. For example, https://a1b2c3.lab.example.net/path gives value=a1b2c3.lab.example.net. Do not shorten it to a parent domain. Use kind=root_domain only for a bare root domain or an explicit request covering all subdomains. Use kind=ip or kind=cidr for an IP address or network range. Leave company-level scope to the later planning stage.

Do not infer additional domains, hosts, or addresses. Give each entry a brief reason tied to the operator's wording. Skip add_task_scope if the task names no asset scope. Register any explicit scope before calling set_goals.`

// GoalSpec is one decomposed objective.
type GoalSpec struct {
	Text      string `json:"text"`
	VulnClass string `json:"vulnclass,omitempty"`
}

// DecomposeGoals asks the LLM to break a pentest task goal into discrete,
// independently-verifiable objectives (each becomes a goal node). Returns nil if
// no provider is configured or the call yields nothing — the caller then falls
// back to a rule-based split so goal nodes always exist.
//
// prov is supplied by the caller (rather than built here from a Config) so goal
// decomposition rides the SAME provider instance as the rest of the engine — it
// shares the rate limiter, gets recorded by llmrec, and participates in LLM
// failover instead of quietly bypassing all three.
//
// desc is the task's free-text description (Background: range of targets/flag Number/Statements of engagement, etc.).
// It is fed alongside the goal so the decomposer no longer splits blind — the
// prompt still forbids inventing anything the two texts don't state.
//
// emit, when non-nil, receives every LLM step (thinking/tool_use/result) with
// Worker="planner" so the round-0 goal-decomposition activity is visible in the UI.
//
// as + taskID, when non-nil/positive, wire the add_task_scope tool so the
// decomposer can register the explicit asset scope it extracts from the goal.
//
// ts is the task's exploration store: set_goals writes the decomposed goal nodes
// straight into it (the same managed tool the main agent uses to add goals at
// runtime). The returned specs are read back from the store so callers can emit
// per-goal activity and detect the "LLM produced nothing" case for their fallback.
func DecomposeGoals(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	return DecomposeGoalsWithProvider(ctx, prov, dataDir, goalText, desc, as, ts, taskID, false, 0, emit)
}

// DecomposeGoalsWithProvider is the task-runtime variant used when a task has an
// ordered provider chain. It preserves the same tools and write behavior while
// letting the caller own provider selection/failover. maxTokens is the profile's
// per-reply output cap (0 = send none).
func DecomposeGoalsWithProvider(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, nonStreaming bool, maxTokens int, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	// Target dismantling is a one-time call: hang up. transcript store,So agentcore I won't. ctx Top
	// session id(It's only there. writer It's late. See you. agentcore.Prompt).And press session-id head
	// Do hint caches/Gateway for sticky routing(opencode zen Missing x-opencode-session Direct 400
	// MissingSessionID)I read it. ctx Value——It's just...[Dialogue normal. Dismantling. 400].
	// Show one steady id:The same explorer's request for dismantling shares it (for Cache) and name and
	// planner/worker No conflict. llmrec.parseSession Correct attribution.
	if ts != nil {
		ctx = transcript.WithSessionID(ctx, fmt.Sprintf("exp%d-goals", ts.ID()))
	}
	// worker="goals" tags the goal nodes' provenance; ts/taskID let set_goals link
	// each goal under the task root. This is the catalog's real set_goals tool, so a
	// web-edited description/schema on it applies here too.
	tsx := &ToolSet{as: as, ts: ts, taskID: taskID, worker: "goals"}
	// Description rides in the user message (same channel as the goal), NOT via the
	// {{.EngagementDescription}} template var — else a prompt that references the var
	// would inject the description twice. System prompt stays pure static instructions.
	sys := renderSystem("goals", goalsDefaultTmpl, GoalsVars{DataDir: dataDir, Now: nowStr()})
	// set_constraints Always available(Not dependent asset store):Text Already Contained[Smuggle operational restraints before target is removed.]This step.
	// (Available at agent Edit Page Reword),All we need here is tools..
	tools := []actool.CoreTool{tsx.setGoals(), tsx.setConstraints()}
	// Wire add_task_scope only when we have a real asset store + task to write to.
	// The scope-extraction tail is appended in lockstep so the prompt never asks for
	// a tool that isn't present.
	if as != nil && taskID > 0 {
		tools = append(tools, tsx.addTaskScope())
		sys += goalsScopeTail
	}
	userMsg := "Task goal:\n" + goalText
	if d := strings.TrimSpace(desc); d != "" {
		userMsg += "\n\nTask description and context, which may include the target scope or engagement terms. Use only what is stated:\n" + d
	}
	// Use captureRun so every LLM step is emitted as an activity record (visible in
	// the plan tab under the round-0 marker). Falls back gracefully when emit is nil.
	captureEmit := func(r db.Activity) {
		if emit != nil {
			r.Worker = "planner"
			emit(r)
		}
	}
	captureRun(ctx, agentcore.Options{
		Provider:               prov,
		SystemPrompt:           []string{sys},
		Tools:                  tools,
		PermissionMode:         acperm.ModeBypass,
		DisableBackgroundTasks: true,
		// 3 Step(Draw constraints → Scope of registration → Targets)One tool call each,Avoid leakage before closing the foot round set_goals.
		MaxTurns:     8,
		NonStreaming: nonStreaming, // The profile Walk when choosing non-stream Provider.Complete
		MaxTokens:    maxTokens,    // 0 = No limit,By the server default
	}, userMsg, captureEmit)
	// set_goals persisted the goals directly; read them back so the caller sees what
	// was written (empty slice ⇒ the LLM produced nothing ⇒ caller falls back).
	if ts == nil {
		return nil
	}
	nodes, _ := ts.ListByKind(db.KindGoal, 10000)
	var out []GoalSpec
	for _, n := range nodes {
		var p struct {
			Text      string `json:"text"`
			VulnClass string `json:"vulnclass"`
		}
		_ = json.Unmarshal(n.Payload, &p)
		if strings.TrimSpace(p.Text) != "" {
			out = append(out, GoalSpec{Text: p.Text, VulnClass: p.VulnClass})
		}
	}
	return out
}
