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

// This document achieves P2[Multi-task Organization Tool Set](docs/Run Division §2 P2).These are... host Tools——Need
// Visits Manager(Arbitrary assignments Store),Engine(Pause),And build task processes,That's why I live here. server Layer.
// Read Tool Set[Existing per-task Tools]Redirect to target mission. store Run!(Build a temporary ToolSet
// and Call Response tool),And then it's exactly the same logic.;Control Class(spawn/pause)Direct Manager/Engine.
// They're like flow tools. seed In. tools Table, by agent Binding(Tie to layout only agent I'll see you there.).

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
	tools = append(tools, s.platformTools()...) // Platform Operating Tool(Construction skill/Tools/MCP,Give Auto Use)
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
		return actool.Errorf("task_id As necessary."), nil
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
	tsx.SetNotify(t.Notify)         // Universal wake-up call (no specific callback to write away; reading tool is no-op)
	tsx.SetNotifyHint(t.NotifyHint) // add_hint → Remember one.[People have added. N A strategic reminder:…]Trigger and wake up planner
	return pick(tsx).Call(ctx, inner, nil)
}

// --- tools ---

func (s *Server) toolListTasks() actool.CoreTool {
	return roTool("list_tasks",
		"List all tasks(id/Description/Target/Status/Running time/Father Job/LLM Configuration),Organization agent Use it to master the situation, see which missions are stuck too long, which ones are used. LLM.Run-time: running=Create→Now, final.=Create→Final activities(second).llm_profile:Task planner/worker The configuration name used,(Activate Configuration)=Follow Global Activation.",
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
					row["llm_profile"] = "(Activate Configuration)"
				} else if n, ok := profName[*llmState.ProfileID]; ok {
					row["llm_profile"] = n
				} else {
					row["llm_profile"] = fmt.Sprintf("#%d(Deleted)", *llmState.ProfileID)
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
		"List available LLM Configuration(profile):id,Name, model, format, current active configuration. Use id Give spawn_task of llm_profile_id Parameters specify sub-task exclusive LLM(For example, the use of cheap models and the use of strong models for reconnaissance). does not contain API Key.",
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
		"Create a task with an independent objective and return its task_id. Supply parent_ref to record its parent task.",
		objSchema(map[string]any{
			"description":            strParam("Short task title"),
			"goal":                   strParam("Task objective"),
			"parent_ref":             strParam("Optional parent task ID"),
			"source_task_ids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": fmt.Sprintf("Optional source task IDs, up to %d. The new task may read their recorded assets and conclusions. parent_ref alone does not add inherited sources.", db.MaxTaskSourceCount)},
			"llm_profile_id":         map[string]any{"type": "integer", "description": "Optional LLM profile ID for the task planner and workers. Omit to inherit the parent or global configuration."},
			"timeout_seconds":        map[string]any{"type": "integer", "description": "Optional task deadline in seconds. Zero or omitted means no deadline."},
			"plan_heartbeat_seconds": map[string]any{"type": "integer", "description": "Optional planner heartbeat interval in seconds; defaults to 600."},
			"seed_first_intent":      map[string]any{"type": "boolean", "description": "Optional direct initial intent for a simple task; defaults to false."},
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
				a.Description = "Unnamed Task"
			}
			if strings.TrimSpace(a.Goal) == "" {
				return actool.Errorf("goal As necessary."), nil
			}
			if a.TimeoutSeconds < 0 {
				a.TimeoutSeconds = 0
			}
			// Inheritance-only mandates: maximum number + each id Valid./Heavy./Existence, verification rules and HTTP The mission is consistent..
			if len(a.SourceTaskIDs) > db.MaxTaskSourceCount {
				return actool.Errorf(fmt.Sprintf("Most selected associated tasks %d pieces", db.MaxTaskSourceCount)), nil
			}
			sourceIDs := make([]int64, 0, len(a.SourceTaskIDs))
			seenSources := map[int64]bool{}
			for _, raw := range a.SourceTaskIDs {
				id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
				if err != nil || id <= 0 || seenSources[id] {
					return actool.Errorf("Associated tasks id Invalid or repeated"), nil
				}
				if _, ok := s.m.Task(strconv.FormatInt(id, 10)); !ok {
					return actool.Errorf(fmt.Sprintf("Associated tasks #%d does not exist", id)), nil
				}
				seenSources[id] = true
				sourceIDs = append(sourceIDs, id)
			}
			// LLM profile resolution: explicit id > inherit parent's pin > active(nil).
			var pin *int64
			if id := parseProfileID(a.LLMProfileID); id > 0 {
				if _, ok := s.loadProfileConfig(id); !ok {
					return actool.Errorf(fmt.Sprintf("LLM Configuration #%d Not available or not set API Key", id)), nil
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
			// Shared Post-Building Processes,With HTTP Construction tasks(server.go createTask)Repeat the same paragraph launchTask:
			// seed + The target is decomposed from the backstage.(No.0wheel/LLMSteps/Article by articlegoal) + engine.Run.
			// seed_first_intent Default false(Standards planned before implementation);A simple task starts with a direct release. work Test.
			s.launchTask(t, a.Description+" "+a.Goal, a.SeedFirstIntent)
			return actool.Text(fmt.Sprintf("task created: %s", t.ID)), nil
		})
}

func (s *Server) toolPauseTask() actool.CoreTool {
	return wrTool("pause_task", "Pause Assignment(Stop it. planner/worker Loop).",
		objSchema(map[string]any{"task_id": strParam("Tasks to suspend id")}, "task_id"),
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
	return roTool("get_task_graph", "Read an overview of the search for specified tasks(Same graph_overview:Asset Count/frontier/Discover/Overwrite etc.),Use task_id Assign Task.",
		objSchema(map[string]any{"task_id": strParam("Task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GraphOverviewTool)
		})
}

func (s *Server) toolListTaskFindings() actool.CoreTool {
	return roTool("list_task_findings", "Can not open message Hole(incl. flag/PoC;Every band. id/task_id/intent_id/vulnclass/severity/Abstract/Status),Use task_id Assign Task.",
		objSchema(map[string]any{"task_id": strParam("Task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListFindingsTool)
		})
}

func (s *Server) toolAddHint() actool.CoreTool {
	return wrTool("add_task_hint", "Infusion of strategic alerts for assigned tasks(Mission planner The next generation will read).\n"+
		"★Priority batch: multi-tip in hints Submit arrays once (return) ids array, with hints Equivalent, Failed id=0);A single article is omitted hints Straight to the top. text.",
		objSchema(map[string]any{
			"task_id":      strParam("Task id"),
			"hints":        map[string]any{"type": "array", "description": "[Take this first.]prompt array, each element field is the top layer(text/asset_ids/traffic_refs).", "items": objSchema(map[string]any{"text": strParam("Note"), "asset_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}, "traffic_refs": agent.HintTrafficSchema()})},
			"text":         strParam("[Single] Note"),
			"traffic_refs": agent.HintTrafficSchema(),
			"asset_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Anchored assets id(Optional,0/1/Multiple; assets within the mandate id)"},
		}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).AddHintTool)
		})
}

func (s *Server) toolGetWorkerTrace() actool.CoreTool {
	return roTool("get_task_worker_trace",
		"Look at one of the assigned tasks. work(Intention)Implementation process:get_task_worker_trace(task_id, intent_id) Read the summary of the steps; take another step_ids=[...] Take those steps.(Most at a time. 5 pieces,More just before you return. 5 pieces).",
		objSchema(map[string]any{
			"task_id":   strParam("Task id"),
			"intent_id": map[string]any{"type": "integer", "description": "Intention id(From the mission. work)"},
			"step_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Optional: Steps to retrieve full content id(Most at a time. 5 pieces,More just before you return. 5 pieces,The rest is here. omitted_step_ids List)"},
		}, "task_id", "intent_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GetWorkerTraceTool)
		})
}

func (s *Server) toolListWorkerTraces() actool.CoreTool {
	return roTool("list_task_worker_traces", "List a task's worker runs and steps. Use get_task_worker_trace for a detailed run.",
		objSchema(map[string]any{"task_id": strParam("Task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListWorkerTracesTool)
		})
}

func (s *Server) toolSearchWorkerTraces() actool.CoreTool {
	return roTool("search_task_worker_traces", "Search all by keyword in the given task work Implementation process(Return hit step summary + intent_id).",
		objSchema(map[string]any{"task_id": strParam("Task id"), "q": strParam("Search keywords")}, "task_id", "q"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).SearchWorkerTracesTool)
		})
}

func (s *Server) toolGetTaskNodeDetail() actool.CoreTool {
	return roTool("get_task_node_detail",
		"Read the full content of an exploratory node in a given task(Discover/fact/Intention/Objective: Summary + Details/Evidence/PoC).id To explore nodes id(As report_finding Return, or list_task_findings inside id).Use it to get full evidence of the loophole before writing the bug report..",
		objSchema(map[string]any{
			"task_id": strParam("Task id"),
			"id":      map[string]any{"type": "integer", "description": "Explore nodes id(Non-assets id)"},
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
		"Write for registered loopholes/Update[Detailed report](Markdown Full text,The whole paragraph overwrites old content).finding_id Pass report_finding Back. id(\"finding recorded: <id>\" Numbers in).Recommendation of the report:Summary of gaps, impacts and hazards, recovery steps, evidence/PoC,Repair suggestions.",
		objSchema(map[string]any{
			"finding_id":       map[string]any{"type": "integer", "description": "Target loophole id(report_finding Returned id)"},
			"report":           strParam("Full detailed report,Markdown Format"),
			"evidence_version": map[string]any{"type": "integer", "description": "get_finding_traffic Evidence of return version;To prevent new evidentiary changes in reporting coverage"},
		}, "finding_id", "report"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				EvidenceVersion *int64          `json:"evidence_version"`
				FindingID       json.RawMessage `json:"finding_id"`
				Report          string          `json:"report"`
			}
			_ = json.Unmarshal(in, &a)
			nodeID := parseProfileID(a.FindingID) // Reuse[Number or number string]Analysis
			if nodeID <= 0 {
				return actool.Errorf("finding_id Invalid"), nil
			}
			n, err := s.m.pg.SetFindingReportVersionByNodeID(ctx, nodeID, a.Report, a.EvidenceVersion)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if n == 0 {
				return actool.Errorf(fmt.Sprintf("Not found finding_id=%d Corresponding loopholes(First. report_finding Register)", nodeID)), nil
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
	// task-op + platform tools default-bind to the built-in Auto agent (It's natural.
	// Operating Platform).SeedTool First insert effective;Coop already seed Other Organiser seedAutoDefaultBindings Tie.
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
	s.seedWorkerReadToolsUnbind() // list_facts/list_companies/list_worker_traces from worker Default untie(One-time)
	s.seedWorkerReadbackRebind()  // Fix old migration error: search_all_worker_traces/get_worker_trace/node_detail Tie back. worker(One-time)
	s.seedAutoReportFindingBinding()
	s.unbindGoalMetDefault()
	s.reseedGoalsPrompt()             // goals Other Organiser[Pump Operating Limit]Step → Add a new version of the old library default(One-time)
	s.reseedMainAgentPrompt()         // mainagent Other Organiser[After the goal is achieved add_intent Asked if we had a target.](One-time)
	s.reseedPlannerPrompt()           // planner Prompt word:Rewrite[0 Intention]Justification + Increased laboratory intake check(One-time)
	s.reseedWorkerPrompt()            // worker Prompt word:Add evidentiary threshold for negative conclusions(One-time)
	s.seedReporterAgent()             // Preset[Report writing]agent + Tool binding + finding Trigger(One-time)
	s.upgradeReporterTriggerMessage() // The old Kuchin move.:Let reporter Reply evidence_version(One-time)
	s.seedFindingTrafficTools()       // Add optional evidentiary parameters and read-only evidence tools to retain user profiles
	s.seedFindingWorkflowTools()
	// Note:pentest Default tool binding does not need to be migrated——BuiltinToolSeeds When the whole new thing starts.
	// list_assets/insert_assets/report_finding/list_findings/list_companies with
	// pentest Together. seed All right.).
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
	// It's also a partial embedding. agent Tool brushes as code default:
	//   - goal_met:Old Library seed . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . planner Think of it as
	//     [Ending the Air Wheel..
	//   - insert_assets:New related Participation(Mark if the asset is relevant to the current task and decides whether to cover it degrees),
	//     SeedTool First Insertonly,Old Library Already seed of schema Otherwise you will not receive this new parameter..
	//   - list_facts:Other Organiser limit/before/q Participation; old library already seed Empty schema Otherwise
	//     Show on Tool Management Page[No parameters],The model doesn't have these parameters..
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
		log.Printf("[tools] goal_met Unbind planner Failed: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// reseedGoalsPrompt handle goals The target demancipator's hint is painted[Current code default]——Because the default body has been added.
// [Draw operational constraints first.(set_constraints)Disable target.]This step.,And SeedPromptIfEmpty First Insertonly,Old Library
// Existing version 1 Can't get this far. Here's the version management.[Add a new version]And cut through.(ResetPromptToDefault),
// The old version is still in history.,Users can be retrieved from the version record if they have defined themselves.settings flag Guard! → Just once.;
// And then the default changes. bump This flag.The whole new library needs no processing.(SeedPromptIfEmpty Already seed Recent Default).
func (s *Server) reseedGoalsPrompt() {
	const flag = "goals_prompt_constraint_step_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // I don't care if I try it once.
	a, err := s.m.pg.GetAgentByKey("goals")
	if err != nil || a == nil {
		return // The whole new library is not built yet. agent Line,seedPrompts It's straight. seed Recent Default,No need to move here
	}
	tmpl := agent.BuiltinPromptSeeds()["goals"]
	if tmpl == "" {
		return
	}
	// New Library seedPrompts Already seed Recent Default → Current version equals code default,No additional copy required.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] goals Quote as new default failed: %v", err)
		return
	}
	log.Printf("[prompts] goals A new default version of the hint has been added(Add a draw-on binding step,One-time)")
}

// reseedMainAgentPrompt handle mainagent Phrasing[Current code default]——Default body added[All Targets
// Once reached add_intent When you vote for intent,,Ask if the person is officially registered.]This direction.,And SeedPromptIfEmpty First Insert
// only,The old library is not available. Manage with Version[Add a new version]And cut through.(ResetPromptToDefault),Old version still
// In history.,Users can be retrieved from the version record if they have defined themselves.settings flag Guard! → Just do it once. The whole new library needs no processing.
// (SeedPromptIfEmpty Already seed Recent Default).With reseedGoalsPrompt Exactly the same..
func (s *Server) reseedMainAgentPrompt() {
	const flag = "mainagent_prompt_goalless_intent_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // I don't care if I try it once.
	a, err := s.m.pg.GetAgentByKey("mainagent")
	if err != nil || a == nil {
		return // The whole new library is not built yet. agent Line,seedPrompts It's straight. seed Recent Default,No need to move here
	}
	tmpl := agent.BuiltinPromptSeeds()["mainagent"]
	if tmpl == "" {
		return
	}
	// New Library seedPrompts Already seed Recent Default → Current version equals code default,No additional copy required.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] mainagent Quote as new default failed: %v", err)
		return
	}
	log.Printf("[prompts] mainagent A new default version of the hint has been added(Once the goal has been achieved, the objective will be answered.,One-time)")
}

// reseedPlannerPrompt handle planner Phrasing[Current code default]——The default body has been streamlined,And put[Restraint.]Downgrade to
// Only heavy, add[Depth over Coverage][Hard Bottom Line:Unachieved objectives and no running intentions required output],To review the negative conclusion.
// Every time there's a change in substance, bump Down there. flag(current v2)Let's do it again..SeedPromptIfEmpty First Insertonly,The old library is not available.,So manage it in version
// [Add a new version]And cut through.(ResetPromptToDefault),The old version is still in history.,Users who have defined themselves can be recorded from the version
// Get it back..settings flag Guard! → Just do it once. The whole new library needs no processing.(SeedPromptIfEmpty Already seed Recent Default).With
// reseedGoalsPrompt Exactly the same..
func (s *Server) reseedPlannerPrompt() {
	const flag = "planner_prompt_compact_realistic_v2"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // I don't care if I try it once.
	a, err := s.m.pg.GetAgentByKey("planner")
	if err != nil || a == nil {
		return // The whole new library is not built yet. agent Line,seedPrompts It's straight. seed Recent Default,No need to move here
	}
	tmpl := agent.BuiltinPromptSeeds()["planner"]
	if tmpl == "" {
		return
	}
	// New Library seedPrompts Already seed Recent Default → Current version equals code default,No additional copy required.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] planner Quote as new default failed: %v", err)
		return
	}
	log.Printf("[prompts] planner A new default version of the hint has been added(Streamlined re-engineering+Repressive downgrading.+Depth priority+Overruled review.,One-time)")
}

// reseedWorkerPrompt handle worker Phrasing[Current code default]——Default Body record_fact It's been deleted.[Negative conclusion
// Write Observation+Experimental reading]The whole sentence, and... confidence(observed/inferred)With[Did you exhaust your means?]Disarm(These susceptible planners.),
// In the meantime, facts Align the arrays to[They're completely independent. They can't be integrated.]Very few exceptions..bump flag To v3 Let's do it again..
// SeedPromptIfEmpty First Insertonly,The old library is not available.,So manage it in version[Add a new version]And cut through.,The old version is still available in history..
// settings flag Guard! → Just do it once. The whole new library needs no processing. and reseedGoalsPrompt Exactly the same..
func (s *Server) reseedWorkerPrompt() {
	const flag = "worker_prompt_compact_v4"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // I don't care if I try it once.
	a, err := s.m.pg.GetAgentByKey("worker")
	if err != nil || a == nil {
		return // The whole new library is not built yet. agent Line,seedPrompts It's straight. seed Recent Default,No need to move here
	}
	tmpl := agent.BuiltinPromptSeeds()["worker"]
	if tmpl == "" {
		return
	}
	// New Library seedPrompts Already seed Recent Default → Current version equals code default,No additional copy required.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] worker Quote as new default failed: %v", err)
		return
	}
	log.Printf("[prompts] worker A new default version of the hint has been added(Check the context as list_assets/list_findings,Get rid of it. list_facts/node_detail/asset_neighbors,One-time)")
}

// reporterToolCallMessage We have to ask unconditionally to read it first. get_finding_traffic Write the report..
// The tool is read-only.,[Not dependent on capture switches],The evidence of artificial binding can be read at all times. If here...
// Written[Enable automatic binding to read],Default close configuration reporter It won't pass. evidence_version,
// SetFindingReportVersionByNodeID Press legacy Semantic -1,Gap Details and Markdown Export
// Other Organiser[Evidence changed, report to be updated],And UI There's no entrance to clear it..
const reporterToolCallMessage = "A finding has been recorded with report_finding. Read its finding_id and finding_node_id from the tool result. " +
	"Call get_finding_traffic with finding_id to read the evidence list and version. If automatic binding is enabled, verify and bind relevant traffic before reporting. " +
	"Read node details with finding_node_id. Save the report with update_finding_report using finding_node_id and the evidence_version returned by get_finding_traffic."

// Old Trigger Message(0.3.8 And sooner.).Only records that are still the same word for word will be migrated to cover, and users will have changed to keep the same..
const reporterToolCallMessageV1 = "There's just a hole in it. report_finding Registration. Please remove from the trigger context finding_id" +
	"(Tool Return \"finding recorded: <id>\" ) and the mission id,Write a detailed report on that loophole in accordance with your duties.," +
	"Last Call update_finding_report(finding_id, report) Save."

// upgradeReporterTriggerMessage The old Curry is still a default. reporter Trigger message as a new version.
// seedReporterAgent Yes. reporter_agent_seed_v1 Guard and only new ones. agent Time-writing trigger, so...
// The upgraded library won't get the new file. —— Tools schema By seedFindingTrafficTools It's done.
// evidence_version,But nothing was told. reporter Go use it. One-time, covering only unaltered texts.
func (s *Server) upgradeReporterTriggerMessage() {
	const flag = "reporter_trigger_evidence_version_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // Just once.
	triggers, err := s.m.pg.ListTriggersFor("reporter")
	if err != nil {
		log.Printf("[reporter] Reading trigger failed: %v", err)
		return
	}
	for _, t := range triggers {
		if !t.OnToolCall || t.ToolCallMessage != reporterToolCallMessageV1 {
			continue // User changed or not finding Trigger, hold it..
		}
		t.ToolCallMessage = reporterToolCallMessage
		if err := s.m.pg.UpdateTrigger(t); err != nil {
			log.Printf("[reporter] Upgrade Trigger Message Failed: %v", err)
			return
		}
		log.Printf("[reporter] Trigger message upgraded to read and return Pass evidence_version")
	}
}

// seedReporterAgent Preset one.[Report writing]Customized agent(builtin=false,Available at UI Edit/Delete):
// Binding update_finding_report + Job Query Tool, and Hang One[report_finding Call or trigger.]of
// Trigger —— Every loophole registered calls for detailed reports. One-time(settings flag Guard!):User delete and not rebuild.
// Dependence:orchestration Tools are above this function SeedTool Enter the library, so it's bound..
func (s *Server) seedReporterAgent() {
	const flag = "reporter_agent_seed_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // I don't care if I try it once.

	if exist, _ := s.m.pg.GetAgentByKey("reporter"); exist != nil {
		return // key Already occupied(User-built)——Do Not Overwrite
	}
	a, err := s.m.pg.CreateAgent("reporter", "Report writer",
		"Draft a detailed Markdown report from recorded finding evidence and execution traces.")
	if err != nil {
		log.Printf("[reporter] Create agent Failed: %v", err)
		return
	}
	if err := s.m.pg.SeedPromptIfEmpty(a.ID, agent.ReporterDefaultPrompt); err != nil {
		log.Printf("[reporter] seed prompt Failed: %v", err)
	}
	// Trigger Run Policy:parallel + none —— One loophole, one report, multiple. finding I'll write it all out..
	// merge Must be. none:Otherwise(Default all)One wave. finding It's going to be combined into a single operation, and there's no point in parallel..
	// maxParallel=5:At the same time. 5 It's a report session. LLM Call.
	if err := s.m.pg.SetAgentTriggerBehavior("reporter", "parallel", "none", 5); err != nil {
		log.Printf("[reporter] Failed to set a trigger running policy: %v", err)
	}
	// The tools it needs to bind: report. + Read the evidence./Execution process/Trends.
	if err := s.m.pg.AddAgentToToolBinding("reporter", []string{
		"update_finding_report", "get_task_node_detail", "list_task_findings",
		"get_task_worker_trace", "list_task_worker_traces", "search_task_worker_traces",
		"get_task_graph",
	}); err != nil {
		log.Printf("[reporter] Failed to bind tool: %v", err)
	}
	// Trigger:report_finding Call or trigger (tool return) "finding recorded: <id>" Take it. finding_id,
	// Task id It's in the trigger.).
	if _, err := s.m.pg.CreateTrigger(&db.AgentTrigger{
		AgentKey:        "reporter",
		Enabled:         true,
		OnToolCall:      true,
		ToolNames:       []string{"report_finding"},
		ToolCallMessage: reporterToolCallMessage,
	}); err != nil {
		log.Printf("[reporter] Failed to create trigger: %v", err)
	}
	log.Printf("[reporter] Configured reporter agent and finding trigger")
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
	const flag = "company_scope_rebind_v1" // worker→planner Default binding switch
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"add_company_scope"}); err != nil {
		log.Printf("[planner] add_company_scope Default binding failed: %v", err)
		return
	}
	if err := s.m.pg.RemoveAgentFromTool("worker", "add_company_scope"); err != nil {
		log.Printf("[worker] add_company_scope Untie failed: %v", err)
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
			log.Printf("[worker] %s from worker Untie failed: %v", k, err)
			return // If you make a mistake, you don't. flag,Try again next time.
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
	const flag = "worker_readback_rebind_v2" // v2: Append node_detail
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("worker", []string{
		"search_all_worker_traces", "get_worker_trace", "node_detail",
	}); err != nil {
		log.Printf("[worker] Look back./Detail tool binding failed: %v", err)
		return // If you make a mistake, you don't. flag,Try again next time.
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedAutoDefaultBindings adds "auto" to the task-op + platform tools' bindings
// ONCE (guarded by a settings flag), so existing DBs whose tool rows were seeded
// before Auto existed still give Auto its default toolset — without re-adding it
// after a user deliberately unbinds.
func (s *Server) seedAutoDefaultBindings() {
	const flag = "auto_default_bindings_v3" // v3: Replace old asset toolnames, add insert_assets/add_company_scope
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	keys := make([]string, 0, len(platformToolKeys)+12)
	for _, t := range s.orchestrationTools() {
		keys = append(keys, t.Name())
	}
	keys = append(keys, platformToolKeys...)
	// Asset tool:Auto The operating platform always looks at it./Register assets, manage the scope of the company.
	keys = append(keys, "insert_assets", "add_company_scope", "list_assets")
	if err := s.m.pg.AddAgentToToolBinding("auto", keys); err != nil {
		log.Printf("[auto] Default binding failed: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}
