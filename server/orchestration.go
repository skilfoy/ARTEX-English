package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/skilfoy/ARTEX-English/agent"
	"github.com/skilfoy/ARTEX-English/db"
)

// jsonResult marshals v to a JSON tool result.
func jsonResult(v any) (actool.Result, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return actool.Errorf(err.Error()), nil
	}
	return actool.Text(string(b)), nil
}

// This file implements the P2 cross-task orchestration toolset (docs/Run Division §2 P2). These are host tools:
// they need the Manager (any task's Store), the Engine (pause), and the task-creation flow, so they live in the server package.
// Read tools point the existing per-task tools at the target task's store (build a temporary ToolSet
// and Call the matching tool), reusing the exact same logic. Control tools (spawn/pause) call the Manager/Engine directly.
// Like the traffic tools, they are seeded into the tools table and bound per agent (only an agent bound to orchestration can see them).

// hostTools is the runtime host-tool provider fed to ToolAugment: traffic tools
// (gated by capture) + cross-task orchestration tools + user-defined custom tools.
// The second return is the names of custom tools flagged `deferred` (schema
// withheld, routed via SearchExtraTools/ExecuteExtraTool). Per-agent binding still
// decides who actually sees any of them.
//
//nolint:unused // used as the hostTools provider in wireAgentAugment
func (s *Server) hostTools() ([]actool.CoreTool, map[string][]string) {
	tools := append(s.m.HostTools(), s.orchestrationTools()...)
	tools = append(tools, s.findingRetestTools()...)
	tools = append(tools, s.platformTools()...) // platform tools (create/update skills, tools, and MCP, for Auto)
	custom, err := s.customTools()
	if err != nil {
		log.Printf("[custom-tool] Loading failed: %v", err)
		return tools, nil
	}
	tools = append(tools, custom...)
	// deferred custom tools → name -> its bound agent keys. ToolAugment turns a
	// name into a deferred entry only for agents it's actually bound to (so we don't
	// advertise a tool the per-agent binding will drop from the callable set).
	deferred := map[string][]string{}
	rows, _ := s.m.pg.ListCustomTools()
	for _, t := range rows {
		if t.Deferred && t.Enabled {
			deferred[t.Key] = t.Agents
		}
	}
	return tools, deferred
}

// orchestrationTools returns the cross-task tool set. Bound per-agent via the
// tools table (default: no binding — opt-in for orchestration agents).
func (s *Server) orchestrationTools() []actool.CoreTool {
	return []actool.CoreTool{
		s.toolListTasks(),
		s.toolListLLMProfiles(),
		s.toolSpawnTask(),
		s.toolPauseTask(),
		s.toolGetTaskGraph(),
		s.toolListTaskFindings(),
		s.toolAddHint(),
		s.toolGetWorkerTrace(),
		s.toolListWorkerTraces(),
		s.toolSearchWorkerTraces(),
		s.toolGetTaskNodeDetail(),
		s.toolUpdateFindingReport(),
		s.toolGetFindingTraffic(),
		s.toolBindFindingTraffic(),
	}
}

// --- schema helpers ---

func strParam(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// parseProfileID reads an LLM profile id from a tool arg that may arrive as a JSON
// number (5) or a numeric string ("5"); returns 0 when absent/unparseable.
func parseProfileID(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		v, _ := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
		return v
	}
	return 0
}

func objSchema(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		req := make([]any, len(required))
		for i, r := range required {
			req[i] = r
		}
		m["required"] = req
	}
	return m
}

func roTool(name, desc string, schema map[string]any, run func(context.Context, json.RawMessage) (actool.Result, error)) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name: name, Description: desc, Schema: schema,
		ReadOnly:   func(json.RawMessage) bool { return true },
		Concurrent: func(json.RawMessage) bool { return true },
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			return run(ctx, in)
		},
	})
}

func wrTool(name, desc string, schema map[string]any, run func(context.Context, json.RawMessage) (actool.Result, error)) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name: name, Description: desc, Schema: schema,
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			return run(ctx, in)
		},
	})
}

// delegateToTask resolves the `task_id` in the input, builds a ToolSet bound to
// that task's store, strips task_id, and calls the chosen per-task tool — so the
// cross-task read reuses the exact in-task logic against another task.
func (s *Server) delegateToTask(ctx context.Context, in json.RawMessage, pick func(*agent.ToolSet) actool.CoreTool) (actool.Result, error) {
	var head struct {
		TaskID string `json:"task_id"`
	}
	_ = json.Unmarshal(in, &head)
	if strings.TrimSpace(head.TaskID) == "" {
		return actool.Errorf("task_id is required"), nil
	}
	t, ok := s.m.Task(head.TaskID)
	if !ok {
		return actool.Errorf("task does not exist: " + head.TaskID), nil
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(in, &m)
	delete(m, "task_id")
	inner, _ := json.Marshal(m)
	tsx := agent.NewToolSet(t.Store, "orchestrator")
	if s.m.Assets() != nil {
		tsx.SetAssetStore(s.m.Assets(), s.m.Assets().Companies())
	}
	tsx.SetNotify(t.Notify)         // generic wake-up (writes with no dedicated callback use it; read tools are a no-op)
	tsx.SetNotifyHint(t.NotifyHint) // add_hint → record "a person added N strategic hints: …", then wake the planner
	return pick(tsx).Call(ctx, inner, nil)
}

// --- tools ---

func (s *Server) toolListTasks() actool.CoreTool {
	return roTool("list_tasks",
		"List every task (id, description, goal, status, runtime, parent task, LLM profile) so an orchestration agent can see the whole picture, which tasks have been stuck, and which LLM each one uses. Runtime: running = created→now; terminal = created→last activity, in seconds. llm_profile is the profile name the task planner and workers use; (active profile) means it follows the global active profile.",
		objSchema(map[string]any{}),
		func(context.Context, json.RawMessage) (actool.Result, error) {
			lastAct, _ := s.m.PG().LastActivityAll()
			// id -> name to resolve each task's pinned LLM profile.
			profName := map[int64]string{}
			if profs, err := s.m.pg.ListProfiles(); err == nil {
				for _, p := range profs {
					profName[p.ID] = p.Name
				}
			}
			out := make([]map[string]any, 0)
			for _, t := range s.m.List() {
				status := s.deriveTaskStatus(t)
				end := lastAct[t.ExpID]
				if live := s.engine.LastActivity(t.ID); live > end {
					end = live
				}
				dur := int64(0)
				if status == "running" {
					dur = time.Now().Unix() - t.CreatedAt
				} else if end > t.CreatedAt {
					dur = end - t.CreatedAt
				}
				row := map[string]any{"id": t.ID, "description": t.Description, "goal": t.Goal, "status": status, "run_seconds": dur}
				if t.ParentRef != "" {
					row["parent_ref"] = t.ParentRef
				}
				llmState := t.llmStateSnapshot()
				if llmState.ProfileID == nil {
					row["llm_profile"] = "(active profile)"
				} else if n, ok := profName[*llmState.ProfileID]; ok {
					row["llm_profile"] = n
				} else {
					row["llm_profile"] = fmt.Sprintf("#%d (deleted)", *llmState.ProfileID)
				}
				out = append(out, row)
			}
			return jsonResult(out)
		})
}

// toolListLLMProfiles lists the available LLM profiles (name/model/active) so an
// orchestration agent can pick one for spawn_task's llm_profile. Never leaks keys.
func (s *Server) toolListLLMProfiles() actool.CoreTool {
	return roTool("list_llm_profiles",
		"List available LLM profiles: id, name, model, format, and whether it is the active profile. Pass id as spawn_task's llm_profile_id to pin a child task to one LLM (a cheap model for recon, a strong model for exploitation). API keys are not included.",
		objSchema(map[string]any{}),
		func(context.Context, json.RawMessage) (actool.Result, error) {
			profs, err := s.m.pg.ListProfiles()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			out := make([]map[string]any, 0, len(profs))
			for _, p := range profs {
				out = append(out, map[string]any{
					"id": p.ID, "name": p.Name, "model": p.Model, "format": p.Format, "is_active": p.IsDefault,
				})
			}
			return jsonResult(map[string]any{"profiles": out})
		})
}

func (s *Server) toolSpawnTask() actool.CoreTool {
	return wrTool("spawn_task",
		"Create a child task, start its exploration engine, and return task_id. Use it to dispatch one piece of work (a challenge or a goal) as its own task. parent_ref is optional: the parent task id of this orchestration run, for the parent-child link.",
		objSchema(map[string]any{
			"description":            strParam("short task title"),
			"goal":                   strParam("what the task should achieve"),
			"parent_ref":             strParam("optional parent task id (parent-child link)"),
			"source_task_ids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": fmt.Sprintf("optional read-only source task ids (at most %d). The child may read those tasks' discovered assets and conclusions as a starting point. Unlike parent_ref, which is only a parent pointer, this inherits content.", db.MaxTaskSourceCount)},
			"llm_profile_id":         map[string]any{"type": "integer", "description": "optional LLM profile id for this child task's planner and workers (see list_llm_profiles). Empty inherits the parent, then falls back to the global active profile."},
			"timeout_seconds":        map[string]any{"type": "integer", "description": "optional task timeout in seconds. When it hits, the task shuts down gracefully and becomes terminal status timeout. Empty or 0 means no limit."},
			"plan_heartbeat_seconds": map[string]any{"type": "integer", "description": "optional planner heartbeat interval in seconds. When this long has passed since the last planning round or task start with no trigger, one planning round runs (deadlock backstop, and a wake to supervise in-flight workers). Empty or 0 means the default 600 (10 min)."},
			"seed_first_intent":      map[string]any{"type": "boolean", "description": "optional. For a simple task, emit one seed intent at creation (text = description + goal) so the worker starts testing without waiting for the first planner round. Default false (plan first, then execute)."},
		}, "description", "goal"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Description          string          `json:"description"`
				Goal                 string          `json:"goal"`
				ParentRef            string          `json:"parent_ref"`
				SourceTaskIDs        []string        `json:"source_task_ids"`
				LLMProfileID         json.RawMessage `json:"llm_profile_id"`
				TimeoutSeconds       int             `json:"timeout_seconds"`
				PlanHeartbeatSeconds int             `json:"plan_heartbeat_seconds"`
				SeedFirstIntent      bool            `json:"seed_first_intent"`
			}
			_ = json.Unmarshal(in, &a)
			if strings.TrimSpace(a.Description) == "" {
				a.Description = "Untitled task"
			}
			if strings.TrimSpace(a.Goal) == "" {
				return actool.Errorf("goal is required"), nil
			}
			if a.TimeoutSeconds < 0 {
				a.TimeoutSeconds = 0
			}
			// Read-only source tasks: cap the count and require each id to be valid, unique, and existing. Same checks as HTTP task creation.
			if len(a.SourceTaskIDs) > db.MaxTaskSourceCount {
				return actool.Errorf(fmt.Sprintf("Select at most %d related tasks", db.MaxTaskSourceCount)), nil
			}
			sourceIDs := make([]int64, 0, len(a.SourceTaskIDs))
			seenSources := map[int64]bool{}
			for _, raw := range a.SourceTaskIDs {
				id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
				if err != nil || id <= 0 || seenSources[id] {
					return actool.Errorf("Related task ids are invalid or duplicated"), nil
				}
				if _, ok := s.m.Task(strconv.FormatInt(id, 10)); !ok {
					return actool.Errorf(fmt.Sprintf("Related task #%d does not exist", id)), nil
				}
				seenSources[id] = true
				sourceIDs = append(sourceIDs, id)
			}
			// LLM profile resolution: explicit id > inherit parent's pin > active(nil).
			var pin *int64
			if id := parseProfileID(a.LLMProfileID); id > 0 {
				if _, ok := s.loadProfileConfig(id); !ok {
					return actool.Errorf(fmt.Sprintf("LLM profile #%d does not exist or has no API key", id)), nil
				}
				pin = &id
			} else if a.ParentRef != "" {
				if pt, ok := s.m.Task(a.ParentRef); ok {
					pin = pt.LLMProfileID
				}
			}
			var llmIDs []int64
			if pin != nil {
				llmIDs = []int64{*pin}
			}
			t, err := s.m.CreateTaskWithOptions(a.Description, a.Goal, db.TaskCreateOptions{
				SourceTaskIDs:        sourceIDs,
				LLMProfileIDs:        llmIDs,
				TimeoutSeconds:       a.TimeoutSeconds,
				PlanHeartbeatSeconds: a.PlanHeartbeatSeconds,
			})
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if a.ParentRef != "" {
				t.ParentRef = a.ParentRef
				if id, e := strconv.ParseInt(t.ID, 10, 64); e == nil {
					_ = s.m.PG().SetParentRef(id, a.ParentRef)
				}
			}
			// Same post-create path as HTTP task creation (server.go createTask), via launchTask:
			// seed, then a visible background goal breakdown (round 0, LLM steps, one goal node each), then engine.Run.
			// seed_first_intent defaults to false (plan first, then execute). A simple task can emit one work item and start testing immediately.
			s.launchTask(t, a.Description+" "+a.Goal, a.SeedFirstIntent)
			return actool.Text(fmt.Sprintf("task created: %s", t.ID)), nil
		})
}

func (s *Server) toolPauseTask() actool.CoreTool {
	return wrTool("pause_task", "Pause a task (stop its planner and worker loops).",
		objSchema(map[string]any{"task_id": strParam("id of the task to pause")}, "task_id"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				TaskID string `json:"task_id"`
			}
			_ = json.Unmarshal(in, &a)
			t, ok := s.m.Task(a.TaskID)
			if !ok {
				return actool.Errorf("task does not exist: " + a.TaskID), nil
			}
			if _, err := s.applyTaskControlWithCause(t, "pause", agent.AbortPausedByOrchestrator); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text("task paused: " + a.TaskID), nil
		})
}

func (s *Server) toolGetTaskGraph() actool.CoreTool {
	return roTool("get_task_graph", "Read the exploration-graph overview of a task (same as graph_overview: asset counts, frontier, findings, coverage). Pass task_id.",
		objSchema(map[string]any{"task_id": strParam("Task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GraphOverviewTool)
		})
}

func (s *Server) toolListTaskFindings() actool.CoreTool {
	return roTool("list_task_findings", "Read confirmed findings for a task (including flag and PoC). Each row has id, task_id, intent_id, vulnclass, severity, summary, and status. Pass task_id.",
		objSchema(map[string]any{"task_id": strParam("Task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListFindingsTool)
		})
}

func (s *Server) toolAddHint() actool.CoreTool {
	return wrTool("add_task_hint", "Inject strategic hints into a task. That task's planner reads them the next time it generates intents.\n"+
		"Prefer a batch: put several hints in the hints array and submit once (returns an ids array, same length and order as hints; a failed item has id 0). For a single hint, omit hints and set the top-level text.",
		objSchema(map[string]any{
			"task_id":      strParam("Task id"),
			"hints":        map[string]any{"type": "array", "description": "preferred: an array of hints. Each element uses the same fields as the top level (text, asset_ids, traffic_refs).", "items": objSchema(map[string]any{"text": strParam("hint text"), "asset_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}, "traffic_refs": agent.HintTrafficSchema()})},
			"text":         strParam("single hint text"),
			"traffic_refs": agent.HintTrafficSchema(),
			"asset_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "asset ids to anchor (optional, zero or more; asset ids inside that task)"},
		}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).AddHintTool)
		})
}

func (s *Server) toolGetWorkerTrace() actool.CoreTool {
	return roTool("get_task_worker_trace",
		"Read one work item (intent) in a task: get_task_worker_trace(task_id, intent_id) returns step summaries; pass step_ids=[...] for the full text of those steps (at most 5; extras are dropped and only the first 5 are returned).",
		objSchema(map[string]any{
			"task_id":   strParam("Task id"),
			"intent_id": map[string]any{"type": "integer", "description": "intent id (a work item in that task)"},
			"step_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "optional step ids whose full content to return (at most 5; extras are not returned and are listed in omitted_step_ids)"},
		}, "task_id", "intent_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GetWorkerTraceTool)
		})
}

func (s *Server) toolListWorkerTraces() actool.CoreTool {
	return roTool("list_task_worker_traces", "List which work items (intents) have run in a task and how many steps each has, so you can see which ones are worth opening with get_task_worker_trace.",
		objSchema(map[string]any{"task_id": strParam("Task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListWorkerTracesTool)
		})
}

func (s *Server) toolSearchWorkerTraces() actool.CoreTool {
	return roTool("search_task_worker_traces", "Search every work item's execution trace in a task by keyword. Returns the matching step summary and intent_id.",
		objSchema(map[string]any{"task_id": strParam("Task id"), "q": strParam("Search keywords")}, "task_id", "q"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).SearchWorkerTracesTool)
		})
}

func (s *Server) toolGetTaskNodeDetail() actool.CoreTool {
	return roTool("get_task_node_detail",
		"Read the full content of one exploration-graph node in a task (finding, fact, intent, or goal: summary plus detail, evidence, and PoC). id is the exploration node id (the id report_finding returned, or the id from list_task_findings). Read it before writing a finding report so the evidence is complete.",
		objSchema(map[string]any{
			"task_id": strParam("Task id"),
			"id":      map[string]any{"type": "integer", "description": "exploration-graph node id (not an asset id)"},
		}, "task_id", "id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).NodeDetailTool)
		})
}

// toolUpdateFindingReport writes/overwrites a finding's detailed Markdown report.
// finding_id is the id report_finding returned ("finding recorded: <id>", the
// finding node id). The write (SetFindingReportByNodeID) is keyed by node_id and
// task-agnostic, so this host tool needs no task_id / exploration store.
func (s *Server) toolUpdateFindingReport() actool.CoreTool {
	return wrTool("update_finding_report",
		"Write or replace the detailed report of a registered finding (full Markdown; the whole text replaces the previous report). finding_id is the id report_finding returned (the number in \"finding recorded: <id>\"). A report should cover the summary, impact, reproduction steps, evidence or PoC, and a fix.",
		objSchema(map[string]any{
			"finding_id":       map[string]any{"type": "integer", "description": "finding id returned by report_finding"},
			"report":           strParam("full report in Markdown"),
			"evidence_version": map[string]any{"type": "integer", "description": "evidence version returned by get_finding_traffic; stops the report from being marked stale when evidence changes"},
		}, "finding_id", "report"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				EvidenceVersion *int64          `json:"evidence_version"`
				FindingID       json.RawMessage `json:"finding_id"`
				Report          string          `json:"report"`
			}
			_ = json.Unmarshal(in, &a)
			nodeID := parseProfileID(a.FindingID) // reuse the "number or numeric string" parser
			if nodeID <= 0 {
				return actool.Errorf("invalid finding_id"), nil
			}
			n, err := s.m.pg.SetFindingReportVersionByNodeID(ctx, nodeID, a.Report, a.EvidenceVersion)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if n == 0 {
				return actool.Errorf(fmt.Sprintf("no finding record for finding_id=%d (register it with report_finding first)", nodeID)), nil
			}
			return actool.Text(fmt.Sprintf("finding %d report updated (%d chars)", nodeID, len(a.Report))), nil
		})
}

// deriveTaskStatus mirrors listTasks' status derivation for the list_tasks tool.
func (s *Server) deriveTaskStatus(t *Task) string {
	lifecycle := t.lifecycleSnapshot()
	switch {
	case isTerminalStatus(lifecycle.Status):
		return lifecycle.Status
	case lifecycle.Paused || s.engine.IsPaused(t.ID):
		return "paused"
	case s.engine.ReadyFor(t) && s.engine.Started(t.ID):
		return "running"
	}
	return "created"
}

// orchestrationToolSeeds seeds the cross-task tools into the tools table so they
// are bindable per-agent (default: bound to nobody — opt-in for orchestration
// agents). First-insert only, like the traffic seeds.
func (s *Server) seedOrchestrationTools() {
	// task-op and platform tools default-bind to the built-in Auto agent (it exists to
	// operate the platform). SeedTool applies on first insert; rows already seeded are bound by seedAutoDefaultBindings.
	autoAgents, _ := json.Marshal([]string{"auto"})
	for _, t := range s.orchestrationTools() {
		schema, _ := json.Marshal(t.InputSchema())
		bindings := autoAgents
		if t.Name() == "bind_finding_traffic" {
			bindings = json.RawMessage(`["reporter"]`)
		}
		_ = s.m.PG().SeedTool(t.Name(), t.Description(), schema, bindings)
	}
	for _, t := range s.platformTools() {
		schema, _ := json.Marshal(t.InputSchema())
		_ = s.m.PG().SeedTool(t.Name(), t.Description(), schema, autoAgents)
	}
	s.refreshBuiltinToolSchemas()
	s.seedAutoDefaultBindings()
	s.seedPlannerDefaultBindings()
	s.seedPlannerListAssetsBinding()
	s.seedCompanyScopeRebind()
	s.seedWorkerReadToolsUnbind() // unbind list_facts, list_companies, and list_worker_traces from worker by default (once)
	s.seedWorkerReadbackRebind()  // fix an old migration that dropped them: bind search_all_worker_traces, get_worker_trace, and node_detail back onto worker (once)
	s.seedAutoReportFindingBinding()
	s.unbindGoalMetDefault()
	s.reseedGoalsPrompt()             // goals prompt: add the "extract operation constraints" step → append a new default on old databases (once)
	s.reseedMainAgentPrompt()         // mainagent prompt: after all goals are met, add_intent asks whether to record a formal goal (once)
	s.reseedPlannerPrompt()           // planner prompt: rewrite the "zero intents" justification and add a quantitative acceptance check (once)
	s.reseedWorkerPrompt()            // worker prompt: add an evidence bar for negative conclusions (once)
	s.seedReporterAgent()             // preset the report-writer agent, its tool bindings, and the finding trigger (once)
	s.upgradeReporterTriggerMessage() // old databases: make the reporter pass evidence_version back (once)
	s.seedFindingTrafficTools()       // Add optional evidentiary parameters and read-only evidence tools to retain user profiles
	s.seedFindingWorkflowTools()
	// pentest's default tool bindings need no migration. BuiltinToolSeeds already seeds
	// list_assets, insert_assets, report_finding, list_findings, and list_companies
	// together with pentest on a fresh init (there is no old database to migrate).
}

// refreshBuiltinToolSchemas propagates code schema/description changes on the
// orchestration + platform tools into already-seeded rows ONCE per version flag —
// SeedTool is first-insert-only, so a new param (e.g. spawn_task of llm_profile) never
// reaches an old DB otherwise. Preserves each tool's agent binding + enabled flag.
// Bump the flag whenever these tools' schemas/descriptions change in code.
func (s *Server) refreshBuiltinToolSchemas() {
	const flag = "tool_schema_refresh_v7_list_facts_paging"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	tools := append(s.orchestrationTools(), s.platformTools()...)
	for _, t := range tools {
		schema, _ := json.Marshal(t.InputSchema())
		if err := s.m.pg.RefreshToolDefaults(t.Name(), t.Description(), schema); err != nil {
			log.Printf("[tools] refresh %s schema failed: %v", t.Name(), err)
		}
	}
	// Also refresh a few built-in agent tools to the code defaults:
	//   - goal_met: the old seeded description said "end this planning round", so the planner
	//     treated it as a way to end an empty round and marked the whole task done right after start.
	//   - insert_assets: new related argument (whether the asset belongs to this task and counts toward coverage).
	//     SeedTool is first-insert only, so an already-seeded schema would never gain the parameter.
	//   - list_facts: now paged, with limit, before, and q. An old empty schema would show "no parameters"
	//     on the tool page, and the model would not see those fields.
	refreshBuiltin := map[string]bool{"goal_met": true, "insert_assets": true, "list_facts": true}
	for _, sd := range agent.BuiltinToolSeeds() {
		if !refreshBuiltin[sd.Key] {
			continue
		}
		schema, _ := json.Marshal(sd.Schema)
		if err := s.m.pg.RefreshToolDefaults(sd.Key, sd.Desc, schema); err != nil {
			log.Printf("[tools] refresh %s desc failed: %v", sd.Key, err)
		}
	}
	_ = s.m.pg.SetSetting(flag, "true")
	log.Printf("[tools] Updated built-in orchestration and platform tool schemas")
}

// unbindGoalMetDefault removes goal_met's default "planner" binding ONCE (guarded by
// a settings flag), so existing DBs match the new default of NO agent. goal_met bypasses
// per-goal prove_goal to declare the whole task done — powerful/risky and redundant with
// the prove_goal→auto-complete path — so it ships unbound; users can re-bind it per agent
// in the UI. A user's own binding to another agent is untouched (we only strip planner).
func (s *Server) unbindGoalMetDefault() {
	const flag = "goal_met_unbind_default_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.RemoveAgentFromTool("planner", "goal_met"); err != nil {
		log.Printf("[tools] failed to unbind goal_met from planner: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// reseedGoalsPrompt refreshes the goals breaker prompt to the current code default. The default
// now says to extract operation constraints (set_constraints) before splitting goals, and
// SeedPromptIfEmpty is first-insert only, so an old version 1 never gets that step. This appends
// a new version and switches to it (ResetPromptToDefault). The old version stays in history so a
// customized prompt can be restored. A settings flag makes it once; bump the flag when the default
// changes again. A fresh database needs nothing (SeedPromptIfEmpty already seeded the latest default).
func (s *Server) reseedGoalsPrompt() {
	const flag = "goals_prompt_constraint_step_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // try once, whether it succeeds or not
	a, err := s.m.pg.GetAgentByKey("goals")
	if err != nil || a == nil {
		return // a fresh database has no agent row yet; seedPrompts writes the latest default, so this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["goals"]
	if tmpl == "" {
		return
	}
	// A fresh database already has the latest default from seedPrompts, so do not append a duplicate version.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to reset the goals prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] goals prompt: appended a new default version (extract operation constraints, once)")
}

// reseedMainAgentPrompt refreshes the mainagent prompt to the current code default. The default now
// says that after every goal is met, an add_intent that posts an intent directly should ask the person
// whether to record it as a formal goal. SeedPromptIfEmpty is first-insert only, so old versions never
// get that guidance. This appends a new version and switches to it (ResetPromptToDefault). The old
// version stays in history. A settings flag makes it once. A fresh database needs nothing. Same shape
// as reseedGoalsPrompt.
func (s *Server) reseedMainAgentPrompt() {
	const flag = "mainagent_prompt_goalless_intent_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // try once, whether it succeeds or not
	a, err := s.m.pg.GetAgentByKey("mainagent")
	if err != nil || a == nil {
		return // a fresh database has no agent row yet; seedPrompts writes the latest default, so this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["mainagent"]
	if tmpl == "" {
		return
	}
	// A fresh database already has the latest default from seedPrompts, so do not append a duplicate version.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to reset the mainagent prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] mainagent prompt: appended a new default version (ask whether to record a goal after goals are met, once)")
}

// reseedPlannerPrompt refreshes the planner prompt to the current code default. The default was
// compacted: "restraint" is now only dedup, depth is preferred over coverage, there is a hard floor
// (an unmet goal with no running intent must still produce output), and negative-conclusion review
// has an upper bound. Bump the flag below (currently v2) whenever the default changes in substance
// so existing databases refresh again. SeedPromptIfEmpty is first-insert only, so this appends a new
// version and switches to it (ResetPromptToDefault). The old version stays in history. Once, via a
// settings flag. A fresh database needs nothing. Same shape as reseedGoalsPrompt.
func (s *Server) reseedPlannerPrompt() {
	const flag = "planner_prompt_compact_realistic_v2"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // try once, whether it succeeds or not
	a, err := s.m.pg.GetAgentByKey("planner")
	if err != nil || a == nil {
		return // a fresh database has no agent row yet; seedPrompts writes the latest default, so this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["planner"]
	if tmpl == "" {
		return
	}
	// A fresh database already has the latest default from seedPrompts, so do not append a duplicate version.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to reset the planner prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] planner prompt: appended a new default version (compact rewrite, restraint reduced to dedup, depth over coverage, cap on negative review, once)")
}

// reseedWorkerPrompt refreshes the worker prompt to the current code default. The record_fact
// section no longer says to write a negative conclusion as an observation plus a tentative reading,
// and confidence (observed/inferred) is decoupled from "did you exhaust this intent's means" (that
// pairing misleads the planner). facts must be fully independent items that cannot be merged, with
// few exceptions. The flag is bumped so existing databases refresh again. SeedPromptIfEmpty is
// first-insert only, so this appends a new version and switches to it; the old version stays in
// history. Once, via a settings flag. A fresh database needs nothing. Same shape as reseedGoalsPrompt.
func (s *Server) reseedWorkerPrompt() {
	const flag = "worker_prompt_compact_v4"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // try once, whether it succeeds or not
	a, err := s.m.pg.GetAgentByKey("worker")
	if err != nil || a == nil {
		return // a fresh database has no agent row yet; seedPrompts writes the latest default, so this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["worker"]
	if tmpl == "" {
		return
	}
	// A fresh database already has the latest default from seedPrompts, so do not append a duplicate version.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to reset the worker prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] worker prompt: appended a new default version (context lookup is list_assets/list_findings; dropped list_facts/node_detail/asset_neighbors, once)")
}

// reporterToolCallMessage must always tell the reporter to read get_finding_traffic before writing.
// The tool is read-only and does not depend on the capture switch, so manually bound evidence is
// readable either way. If the text said "read only when automatic binding is on", a default-off
// reporter would omit evidence_version, SetFindingReportVersionByNodeID would store the legacy -1,
// and the finding detail and Markdown export would stay on "evidence changed, report needs update"
// with no UI control to clear it.
const reporterToolCallMessage = "A finding was just registered with report_finding. Read finding_id (the finding record id) and finding_node_id (the exploration node id) from the returned JSON. " +
	"Call get_finding_traffic(finding_id) first and read the evidence list and its version. An empty list is normal; still write the report. " +
	"If the run instructions enable automatic binding, verify and bind this finding's traffic before reading. Use finding_node_id for node detail. " +
	"Then call update_finding_report with finding_id set to finding_node_id, the report, and evidence_version set to the version you actually read. " +
	"evidence_version is required; without it the report is permanently marked as needing an update. Do not mix the two ids."

// Previous trigger message (0.3.8 and earlier). Only a record that still matches this text byte for byte is overwritten; a message the user edited is left alone.
// The string below is a migration sentinel. Do not reword it or existing rows will no longer match.
const reporterToolCallMessageV1 = "There's just a hole in it. report_finding Registration. Please remove from the trigger context finding_id" +
	"(Tool Return \"finding recorded: <id>\" ) and the mission id,Write a detailed report on that loophole in accordance with your duties.," +
	"Last Call update_finding_report(finding_id, report) Save."

// upgradeReporterTriggerMessage rewrites reporter trigger messages that are still the old default.
// seedReporterAgent is guarded by reporter_agent_seed_v1 and writes the trigger only when the agent
// is created, so an upgraded database never receives the new text. seedFindingTrafficTools added the
// evidence_version schema, but nothing told the reporter to pass it. Once, and only untouched text.
func (s *Server) upgradeReporterTriggerMessage() {
	const flag = "reporter_trigger_evidence_version_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // try once
	triggers, err := s.m.pg.ListTriggersFor("reporter")
	if err != nil {
		log.Printf("[reporter] failed to read triggers: %v", err)
		return
	}
	for _, t := range triggers {
		if !t.OnToolCall || t.ToolCallMessage != reporterToolCallMessageV1 {
			continue // the user edited it, or it is not the finding trigger; leave it
		}
		t.ToolCallMessage = reporterToolCallMessage
		if err := s.m.pg.UpdateTrigger(t); err != nil {
			log.Printf("[reporter] failed to upgrade the trigger message: %v", err)
			return
		}
		log.Printf("[reporter] trigger message upgraded to read traffic and pass evidence_version")
	}
}

// seedReporterAgent presets a custom "report writer" agent (builtin=false, editable and deletable in the UI).
// It binds update_finding_report and the task-query tools, and hangs a trigger that fires when
// report_finding is called, so each registered finding gets a detailed report. Once (settings flag):
// if the user deletes it, it is not recreated. The orchestration tools are SeedTool'd above, so the bind succeeds.
func (s *Server) seedReporterAgent() {
	const flag = "reporter_agent_seed_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // try once, whether it succeeds or not

	if exist, _ := s.m.pg.GetAgentByKey("reporter"); exist != nil {
		return // key is already taken (a user created it); do not overwrite
	}
	a, err := s.m.pg.CreateAgent("reporter", "Report writer",
		"Writes a detailed finding report: triggered when a finding is recorded, reads the evidence and execution trace, then writes a Markdown report back.")
	if err != nil {
		log.Printf("[reporter] failed to create agent: %v", err)
		return
	}
	if err := s.m.pg.SeedPromptIfEmpty(a.ID, agent.ReporterDefaultPrompt); err != nil {
		log.Printf("[reporter] failed to seed prompt: %v", err)
	}
	// Trigger run policy: parallel + none — one finding, one report, and several findings write at once.
	// merge must be none. The default all would fold a burst of findings into one run, which makes parallel pointless.
	// maxParallel=5: at most 5 report sessions at once, so a burst does not fan out into too many LLM calls.
	if err := s.m.pg.SetAgentTriggerBehavior("reporter", "parallel", "none", 5); err != nil {
		log.Printf("[reporter] failed to set the trigger run policy: %v", err)
	}
	// Tools it needs: write the report, and read evidence, the execution trace, and the picture.
	if err := s.m.pg.AddAgentToToolBinding("reporter", []string{
		"update_finding_report", "get_task_node_detail", "list_task_findings",
		"get_task_worker_trace", "list_task_worker_traces", "search_task_worker_traces",
		"get_task_graph",
	}); err != nil {
		log.Printf("[reporter] failed to bind tools: %v", err)
	}
	// Trigger: fires when report_finding is called. The tool result "finding recorded: <id>" carries
	// finding_id, and the task id is in the trigger message too.
	if _, err := s.m.pg.CreateTrigger(&db.AgentTrigger{
		AgentKey:        "reporter",
		Enabled:         true,
		OnToolCall:      true,
		ToolNames:       []string{"report_finding"},
		ToolCallMessage: reporterToolCallMessage,
	}); err != nil {
		log.Printf("[reporter] failed to create trigger: %v", err)
	}
	log.Printf("[reporter] preset the report-writer agent and its finding trigger")
}

// seedAutoReportFindingBinding adds "auto" to report_finding's binding ONCE so
// conversation-context agents can call it without requiring an intent_id.
func (s *Server) seedAutoReportFindingBinding() {
	const flag = "auto_report_finding_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("auto", []string{"report_finding"}); err != nil {
		log.Printf("[auto] report_finding Default binding failed: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedPlannerDefaultBindings adds "planner" to report_finding's binding ONCE
// (guarded by a settings flag), so existing DBs — whose report_finding row was
// seeded as worker-only — also let the planner record findings. Fresh DBs already
// get it via PlannerTools(); this only backfills without overriding a user unbind.
func (s *Server) seedPlannerDefaultBindings() {
	const flag = "planner_report_finding_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"report_finding"}); err != nil {
		log.Printf("[planner] report_finding Default binding failed: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedPlannerListAssetsBinding adds "planner" to list_assets's binding ONCE
// (guarded by a settings flag), so existing DBs — whose list_assets row was seeded
// as auto/pentest-only — also let the planner query the asset store by DSL. Fresh
// DBs already get it via PlannerTools(); this only backfills without overriding a
// user unbind.
func (s *Server) seedPlannerListAssetsBinding() {
	const flag = "planner_list_assets_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"list_assets"}); err != nil {
		log.Printf("[planner] list_assets Default binding failed: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedCompanyScopeRebind changes add_company_scope's default binding ONCE on
// existing DBs (guarded by a settings flag): the tool moves off worker and onto
// planner — defining a company's asset scope is a planning/main/auto concern, not
// something a worker does mid-exploration. Fresh DBs already get planner via
// PlannerTools() and lack worker via WorkerTools(); this only backfills old rows.
// One-shot + flag-guarded so a user who later re-binds worker isn't overridden.
func (s *Server) seedCompanyScopeRebind() {
	const flag = "company_scope_rebind_v1" // one-time worker→planner default binding move
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"add_company_scope"}); err != nil {
		log.Printf("[planner] add_company_scope Default binding failed: %v", err)
		return
	}
	if err := s.m.pg.RemoveAgentFromTool("worker", "add_company_scope"); err != nil {
		log.Printf("[worker] failed to unbind add_company_scope: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedWorkerReadToolsUnbind strips the read-context tools off worker's default
// binding ONCE on existing DBs (guarded by a settings flag): a worker executes one
// intent and writes back — reading facts/companies and listing all workers' traces is
// a planning/main concern, not the executor's. Fresh DBs already lack these via
// WorkerTools(); this only backfills old rows without overriding a user who
// deliberately re-binds worker. Each RemoveAgentFromTool is per-tool +
// membership-guarded, so planner/mainagent bindings of the same tool are untouched.
//
// NOTE: search_all_worker_traces / get_worker_trace / node_detail are intentionally NOT
// unbound — worker owns them for cross-work look-back + node drill-down (see WorkerTools).
// They used to be in this list back when worker lacked them; seedWorkerReadbackRebind
// repairs DBs whose old run stripped them.
func (s *Server) seedWorkerReadToolsUnbind() {
	const flag = "worker_readtools_unbind_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	for _, k := range []string{
		"list_facts", "list_companies", "list_worker_traces",
	} {
		if err := s.m.pg.RemoveAgentFromTool("worker", k); err != nil {
			log.Printf("[worker] failed to unbind %s from worker: %v", k, err)
			return // on error, do not set the flag, so the next startup retries
		}
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedWorkerReadbackRebind re-binds the cross-work look-back / drill-down tools onto
// worker ONCE (guarded by a settings flag): an earlier seedWorkerReadToolsUnbind wrongly
// stripped search_all_worker_traces / get_worker_trace / node_detail from worker after
// they had been added to WorkerTools(), so any DB that ran that migration lost them.
// Fresh DBs already have them via WorkerTools() and this is a harmless no-op there.
// One-shot + flag-guarded so a user who later deliberately unbinds them isn't overridden.
func (s *Server) seedWorkerReadbackRebind() {
	const flag = "worker_readback_rebind_v2" // v2: also bind node_detail
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("worker", []string{
		"search_all_worker_traces", "get_worker_trace", "node_detail",
	}); err != nil {
		log.Printf("[worker] failed to bind look-back and detail tools: %v", err)
		return // on error, do not set the flag, so the next startup retries
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedAutoDefaultBindings adds "auto" to the task-op + platform tools' bindings
// ONCE (guarded by a settings flag), so existing DBs whose tool rows were seeded
// before Auto existed still give Auto its default toolset — without re-adding it
// after a user deliberately unbinds.
func (s *Server) seedAutoDefaultBindings() {
	const flag = "auto_default_bindings_v3" // v3: drop old asset tool names; add insert_assets and add_company_scope
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	keys := make([]string, 0, len(platformToolKeys)+12)
	for _, t := range s.orchestrationTools() {
		keys = append(keys, t.Name())
	}
	keys = append(keys, platformToolKeys...)
	// Asset tools: Auto runs the platform, so it always lists and registers assets and manages company scope.
	keys = append(keys, "insert_assets", "add_company_scope", "list_assets")
	if err := s.m.pg.AddAgentToToolBinding("auto", keys); err != nil {
		log.Printf("[auto] Default binding failed: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}
