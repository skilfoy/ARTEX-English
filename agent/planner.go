package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
	"github.com/skilfoy/ARTEX-English/db"
	"github.com/skilfoy/ARTEX-English/intercept"
)

// Planner is the event-driven LLM planner (docs §4.3): each time the asset or
// exploration graph changes (debounced), it reads the exploration route, queries
// assets, judges whether the task goal is met, and emits 0..N exploration intents
// into the frontier. It is the sole intent generator.
type Planner struct {
	findingRecorder   FindingRecorder
	prov              llm.Provider
	model             string
	tx                *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window            int                                    // context window in tokens (for compaction)
	windowFn          func() int                             // optional dynamic task-chain minimum
	maxTurns          int                                    // max agent turns per run (0 = unlimited)
	killWork          func(intentID int64) error             // engine callback to terminate a running work (nil = off)
	steerWork         func(intentID int64, msg string) error // engine callback to steer a running work mid-run (nil = off)
	proxyAddr         string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert       string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch         WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir           string                                 // shared work dir (surfaced in prompt as artifact-output target)
	injectConstraints func() bool                            // resolver: inject task operation constraints into system prompt? (nil = yes)
	nonStreamingFn    func() bool                            // resolver: use non-streaming (Complete) path? (nil = streaming)
	noaEnabledFn      func() bool                            // resolver: use experimental noa compaction? (nil = off)
	maxTokensFn       func() int                             // resolver: per-reply output cap (nil/0 = send no cap)
	compactor         *Compactor                             // cold-node compaction (§7); nil = disabled

	// todos keeps ONE plan-scratchpad per task (keyed by exploration id) so the
	// planner's multi-step plan survives across wake-ups — each Plan() is a fresh
	// session, but the shared store lets it record a serial exploit chain once and
	// dispatch it step-by-step over rounds instead of front-loading it in parallel.
	todoMu sync.Mutex
	todos  map[int64]*actool.TodoStore
}

func NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner {
	return &Planner{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns, todos: map[int64]*actool.TodoStore{}}
}

func (p *Planner) SetCompactionWindowResolver(fn func() int) { p.windowFn = fn }

// SetCompactor wires the cold-node compactor (cold-digest §7). Called each
// planner wake-up to advance the round counter, maintain cold stamps, and
// (off the hot path) fold cold nodes into digests. nil = feature disabled.
func (p *Planner) SetCompactor(c *Compactor) { p.compactor = c }

// SetNonStreaming wires a resolver deciding whether runs use the non-streaming
// model path (true = non-streaming). nil/unset = streaming (default).
func (p *Planner) SetNonStreaming(fn func() bool) { p.nonStreamingFn = fn }

func (p *Planner) nonStreaming() bool { return p.nonStreamingFn != nil && p.nonStreamingFn() }

// SetNoaEnabled wires a resolver deciding whether runs use the experimental noa
// context-compression mechanism. nil/unset = off (built-in compaction). Read per
// run so the settings toggle takes effect without rebuilding the agent.
func (p *Planner) SetNoaEnabled(fn func() bool) { p.noaEnabledFn = fn }

// SetMaxTokens wires a resolver for the per-reply output cap. nil/unset or 0 =
// send no cap and let the endpoint decide. Read per run, like nonStreaming.
func (p *Planner) SetMaxTokens(fn func() int) { p.maxTokensFn = fn }

func (p *Planner) maxTokens() int {
	if p.maxTokensFn == nil {
		return 0
	}
	return p.maxTokensFn()
}

func (p *Planner) compactionWindow() int {
	if p.windowFn != nil {
		return p.windowFn()
	}
	return p.window
}

// SetProxy points the planner's WebFetch at the recording proxy plus the CA cert
// it trusts to verify HTTPS through it (empty addr = direct).
func (p *Planner) SetProxy(addr, caCert string) { p.proxyAddr, p.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the planner (off by default).
func (p *Planner) SetWebSearch(o WebSearchOpts) { p.webSearch = o }

// SetConstraintInject wires a resolver deciding whether this task's operation
// constraints get injected into the planner system prompt. Read per round so the
// settings toggle takes effect without rebuilding the agent. nil = inject (default).
func (p *Planner) SetConstraintInject(fn func() bool) { p.injectConstraints = fn }

// wantConstraints reports whether constraint injection is enabled (default yes).
func (p *Planner) wantConstraints() bool { return p.injectConstraints == nil || p.injectConstraints() }

// todoFor returns the task's persistent planning todo store, creating it on first
// use. Shared across all of this task's planner wake-ups.
func (p *Planner) todoFor(expID int64) *actool.TodoStore {
	p.todoMu.Lock()
	defer p.todoMu.Unlock()
	s := p.todos[expID]
	if s == nil {
		s = actool.NewTodoStore()
		p.todos[expID] = s
	}
	return s
}

// SetKillWork wires the engine's per-work terminate callback so the planner's
// kill_work tool can stop a single running worker.
func (p *Planner) SetKillWork(fn func(intentID int64) error) { p.killWork = fn }

// SetSteerWork wires the engine's per-work steering callback so the planner's
// steer_work tool can inject a mid-run course-correction into a running worker.
func (p *Planner) SetSteerWork(fn func(intentID int64, msg string) error) { p.steerWork = fn }

// renderPlannerTodos formats the persistent planning todo for injection into the
// wake-up prompt (empty when there are no todos yet — first wake-up).
func renderPlannerTodos(items []actool.Todo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nPlanning to-do list retained from the previous round:\n")
	for _, it := range items {
		mark := map[actool.TodoStatus]string{actool.TodoPending: "☐", actool.TodoInProgress: "▶", actool.TodoCompleted: "✔"}[it.Status]
		if mark == "" {
			mark = "☐"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, it.Content))
	}
	b.WriteString("Dispatch the next intent only after its prerequisites are complete and the required facts exist. Use TodoWrite to mark completed steps. Do not dispatch a pending or active step again.")
	return b.String()
}

// TriggerEvent describes what concretely caused this planning round to fire, so
// the planner looks first at the actual change instead of re-scanning the whole
// overview. Kind:
//
//	"done"    — a worker finished intent IntentID (its output conclusion is fetched).
//	"finding" — a worker reported a finding on intent IntentID (Detail = Abstract).
//	"goal"    — the human (via Lord agent of set_goals) added one OR MORE goals in a
//	            single call (Goals = This new target text,1+ strip;set_goals Support batch).
//	"goal_deleted" — the human deleted a goal from Objective management of the overview (Detail = Deleted target text).
//	"goal_edited"  — the human edited a goal from Objective management of the overview (OldGoal→NewGoal Text).
//	"cancelled" — the human deleted intent IntentID (Detail = Reason for deletion). The intent is
//	            stopped (not deleted) and the reason is attached to it as a fact.
type TriggerEvent struct {
	Kind     string
	IntentID int64
	Detail   string
	Summary  string   // Kind=="cancelled" Earmarked: Summary of intent captured before deletion (no longer available, no longer available))
	Goals    []string // Kind=="goal" Earmarked: This time set_goals New Target Text(1 Articles or articles)
	OldGoal  string   // Kind=="goal_edited" Earmarked: Target text before change
	NewGoal  string   // Kind=="goal_edited" Earmarked: modified target text
	Hints    []string // Kind=="hint" Earmarked: This time add_hint New Hint text(1 Articles or articles)
}

// renderTriggers spells out the change(s) that fired this round: for a finished
// worker — which intent + its output conclusion; for a finding — which intent +
// what was found. Empty for time/heartbeat wakes. Reads the store (best-effort;
// a blank field never blocks the round).
func renderTriggers(ts *db.ExplorationStore, evs []TriggerEvent) string {
	if len(evs) == 0 || ts == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nChanges that triggered this planning round (review these first):")
	for _, ev := range evs {
		switch ev.Kind {
		case "goal":
			if len(ev.Goals) == 1 {
				b.WriteString(fmt.Sprintf("\n- The operator added a goal: %s. Add an exploration direction if no intent covers it.", ev.Goals[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The operator added %d goals: %s. Review each for an uncovered direction.", len(ev.Goals), strings.Join(ev.Goals, "; ")))
			}
		case "hint":
			if len(ev.Hints) == 1 {
				b.WriteString(fmt.Sprintf("\n- The operator added a strategic hint: %s. Adjust the plan where appropriate.", ev.Hints[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The operator added %d strategic hints: %s. Review each when planning.", len(ev.Hints), strings.Join(ev.Hints, "; ")))
			}
		case "goal_deleted":
			b.WriteString(fmt.Sprintf("\n- The operator removed this goal: %s. Reassess the remaining goals and do not dispatch further work for the removed goal.", ev.Detail))
		case "goal_edited":
			b.WriteString(fmt.Sprintf("\n- The operator changed a goal from %q to %q. Adjust the plan and stop dispatching obsolete directions.", ev.OldGoal, ev.NewGoal))
		case "finding":
			b.WriteString(fmt.Sprintf("\n- The worker on intent #%d (%s) reported a finding: %s", ev.IntentID, intentSummary(ts, ev.IntentID), ev.Detail))
		case "cancelled":
			// Intentional content preferred to be captured when deleted Summary(Really delete the postnode does not exist,intentSummary I can't.).
			sm := ev.Summary
			if sm == "" {
				sm = intentSummary(ts, ev.IntentID)
			}
			b.WriteString(fmt.Sprintf("\n- The operator removed intent #%d (%s). Reason: %s. Replan without this intent.", ev.IntentID, sm, ev.Detail))
		default: // "done"
			b.WriteString(fmt.Sprintf("\n- The worker on intent #%d (%s) finished. Conclusion: %s", ev.IntentID, intentSummary(ts, ev.IntentID), workerOutput(ts, ev.IntentID)))
			if fids := factIDsYielded(ts, ev.IntentID); fids != "" {
				b.WriteString(fmt.Sprintf(". New fact IDs: %s ", fids))
			}
		}
	}
	b.WriteString("\nUse node_detail, get_worker_output, or list_findings for complete details.")
	return b.String()
}

// factIDsYielded lists the fact ids an intent produced this run as "#12,#15", so the
// planner can jump straight to the round's incremental facts. Empty (best-effort) when
// the intent yielded no facts or the lookup fails.
func factIDsYielded(ts *db.ExplorationStore, id int64) string {
	ids, err := ts.FactsYielded(id)
	if err != nil || len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, fid := range ids {
		parts[i] = fmt.Sprintf("#%d", fid)
	}
	return strings.Join(parts, ",")
}

// intentSummary reads an intent node's one-line summary (best-effort, "?" on miss).
func intentSummary(ts *db.ExplorationStore, id int64) string {
	n, err := ts.GetNode(id)
	if err != nil || n == nil {
		return "?"
	}
	var p map[string]any
	if json.Unmarshal(n.Payload, &p) == nil {
		if s, ok := p["summary"].(string); ok && s != "" {
			return s
		}
	}
	return "?"
}

// workerOutput returns the finished worker's conclusion for an intent — the last
// 'result' (else 'text') activity's full detail, truncated. Same source get_worker_output uses.
func workerOutput(ts *db.ExplorationStore, id int64) string {
	acts, _, err := ts.ActivityList(&id, 0, 1000)
	if err != nil {
		return "(could not retrieve worker output)"
	}
	var pick *db.Activity
	for i := range acts {
		if acts[i].Kind == "result" {
			pick = &acts[i]
		} else if acts[i].Kind == "text" && pick == nil {
			pick = &acts[i]
		}
	}
	if pick == nil {
		return "(no worker output recorded yet)"
	}
	out, _ := ts.ActivityDetail(pick.ID)
	if out == "" {
		out = pick.Summary
	}
	return truncOutput(out, 800)
}

// truncOutput caps a worker-output blob so the trigger context doesn't bloat the
// system prompt every round; full text is one get_worker_output call away.
func truncOutput(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " ... (truncated; use get_worker_output for the full text)"
}

// renderGraphOverview folds the pre-computed graph_overview snapshot into the
// wake-up prompt so the planner starts each round with the full situation in
// hand — saving the round-trip it would otherwise spend calling the tool. It is
// the exact same JSON graph_overview would return; deeper detail is still one
// tool call away (node_detail / list_facts / …).
func renderGraphOverview(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "" // fall back to the model calling graph_overview itself
	}
	return "\n\nCurrent task state (prefetched graph_overview; use node_detail or list_facts for detail):\n" + string(b)
}

// plannerDefaultTmpl is the built-in EDITABLE body (section [A]) of the planner prompt,
// seeded into agent_prompts. Goal is a {{.Goal}} template var; the Intermediate output regulations
// tail is code-owned (artifactSpec) and appended by plannerSystem after rendering.
const plannerDefaultTmpl = `You are the planner for an authorized security assessment. Review the task graph, assess its goals, and create exploration intents for uncovered directions. Workers execute those intents. Respond in English.

Task goal: {{.Goal}}

Use the graph_overview supplied below as the current task state. Its goals, intents, facts, findings, and recent activity belong to this task. Assets are shared across tasks, so consider only assets relevant to this task. Read node_detail, list_facts, list_findings, list_assets, get_worker_trace, or get_worker_output when a decision requires detail beyond the overview.

Assess every unmet goal against recorded evidence. Use prove_goal(goal_id, evidence_id, reason) only after the outcome has been verified. Apply quantitative thresholds exactly. The task completes through individual prove_goal calls for its final goals.

Create intents according to these rules:
- If any goal remains unmet and there are no open or running intents, submit at least one intent that advances an unmet goal. Review incomplete prior work and find a feasible direction within scope. Never leave an unfinished task idle without an intent.
- If open or running intents already cover every relevant direction, wait for their results. Avoid new intents that merely restate work already assigned.
- A completed intent can be revisited only with a material new mechanism, asset, parameter, or evidence. Inspect its output before resuming an intent that exhausted its budget or failed externally.
- Treat a negative observation as provisional until its evidence and confidence support the conclusion. One independent review may resolve a weak negative result. Respect a supported negative result after review.
- Keep materially different routes available while the goal remains unmet. Prioritize depth along a promising, evidenced route. Stay within every operation constraint.
- Represent a dependent sequence in TodoWrite and dispatch its steps in order, after each prerequisite has produced the required fact. Mark satisfied steps complete. Independent work can run in parallel.

At the beginning of a task, a minimal read-only check can clarify an initial direction if the graph has no worker facts. Stop such checks as soon as an intent can be stated. Give substantive testing to workers.

Submit up to four highest-value new directions in one add_intent call. Each summary should identify the target, intended investigation, and reason. Include relevant asset_ids and parent_ids where available. A round can submit zero intents when current work covers the directions or a prerequisite is pending. Explain that choice briefly and accurately.`

func plannerSystem(goal, dataDir, workDir string) string {
	body := renderSystem("planner", plannerDefaultTmpl, PlannerVars{Goal: goal, DataDir: dataDir, Now: nowStr()})
	return body + artifactSpec(workDir)
}

// Plan runs one planning round. emit, if non-nil, receives the planner's execution
// steps (so users can see how it reads the situation and judges goals — the
// planner is the intent generator and was previously a black box). Returns whether
// the planner judged the goal met.
// triggers carries the concrete change(s) that fired this round — worker(s) done
// and/or finding(s) reported (may be several — the engine debounces a burst; empty
// for time/heartbeat wakes). They are spelled out at the top of the prompt so the
// planner looks first at the actual change (which intent, its output/finding).
func (p *Planner) Plan(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal string, triggers []TriggerEvent, emit func(db.Activity)) (met bool, reason string, err error) {
	// cold-digest §2.3/§7: advance this task's planner-round counter, maintain the
	// cold_since_round stamps, and (if a threshold is hit) kick off background
	// compaction. Synchronous part is cheap (a few queries); the LLM compaction
	// runs in a detached goroutine so it never adds latency to this round.
	p.compactor.OnPlannerRound(ctx, ts)
	tsx := NewToolSet(ts, "planner")
	tsx.SetFindingRecorder(p.findingRecorder)
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetCoverageEnabled(as == nil || as.CoverageEnabled(taskID))
	tsx.killWork = p.killWork   // enable kill_work tool (nil = unavailable)
	tsx.steerWork = p.steerWork // enable steer_work tool (nil = unavailable)
	if origin, _ := ts.OriginFactID(); origin > 0 {
		tsx.SetOwnerNode(origin) // planner-side anchors default to the task root (origin fact)
	}
	// Field tools + Basic Default Toolset(Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash)
	// Remove asset overwhelm functionality when off add_task_scope/list_untested_assets(I can't. prompt).
	base := append(tsx.DropCoverageTools(tsx.PlannerTools()), actool.DefaultTools()...)
	ctx = WithRunInfo(ctx, RunInfo{TaskID: taskID, ExplorationID: explorationID(ts)})
	tools, def, cleanup := AugmentTools(ctx, "planner", base)
	defer cleanup()
	// Critical postures (intent just completed) + Full preset)[Current round user Input](See below input),system
	// Only static planning body.move-out Let system Each round is more stable and more resilient; the cost is that if a single wheel is longer, the posture may
	// Being compaction Compression(planner Single rounds are usually short and low-risk.).situational It's going down. input.
	situational := renderTriggers(ts, triggers) + renderGraphOverview(tsx.graphOverviewData())
	// Task level deadline / Final Mode(Sutra ctx Injection,See taskclock.go).The final round timed off.
	// planner End of sentence as[Current round of operational instructions]Spell in the current wheel user Input(Whatever. situational),Make it last.
	// Targeting, no new intent..
	tc := taskClockFrom(ctx)
	if tc.Final {
		situational += "\n\nFinal task wrap-up instruction for this round: " + resolveTaskTimeoutWrapup("planner")
	}
	// Task Directory <workDir>/tasks/<taskID>,Build it first..
	taskDir := ensureRunDir(p.workDir, taskID, 0)
	ctx = intercept.WithReviewContext(ctx, taskDir, intercept.ReviewBackground{})
	sysBody := plannerSystem(goal, p.workDir, taskDir)
	if p.wantConstraints() {
		sysBody += constraintBlock(ts) // Operational constraints(If there is.)Injection system hint,Framed Explored Borders
	}
	system, boundary := deferredSystem(sysBody, def)
	// planner No own wall budget;Yes deadline Time MaxDuration We'll get the rest.,Let the planning wheel on the run
	// Finish the task at point.(Because it's time out.→Task timeout,Step→per-run Words).
	maxDur, clamped := clampMaxDuration(tc.DeadlineUnix, 0)
	settle := wrapupSettlement("planner", nil)
	if tc.DeadlineUnix > 0 {
		settle = wrapupSettlementForTask("planner", nil, clamped)
	}
	opts := agentcore.Options{
		Provider:        p.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, // Walking records agent leaves marks;loading agent CA Verification MITM Resigned HTTPS Certificate
		WebFetchProxy:   p.proxyAddr,
		WebFetchCACert:  p.proxyCACert,
		// Network Search(Optional).ddgs No need key;brave-free Required BraveKey;tavily Required TavilyKey.
		// WebSearchProxy It's an independent export agent.(http/https/socks5),With the traffic recorded MITM Agent is irrelevant; empty is direct.
		EnableWebSearch:       p.webSearch.Enabled,
		WebSearchBackend:      p.webSearch.Backend,
		BraveSearchAPIKey:     p.webSearch.BraveKey,
		TavilySearchAPIKey:    p.webSearch.TavilyKey,
		DeepSeekSearchBaseURL: p.webSearch.DeepSeekBaseURL,
		DeepSeekSearchAPIKey:  p.webSearch.DeepSeekAPIKey,
		DeepSeekSearchModel:   p.webSearch.DeepSeekModel,
		WebSearchProxy:        p.webSearch.Proxy,
		BashEnv:               proxyEnv(p.proxyAddr, p.proxyCACert), // Bash Sub-order default proxy+Trust CA
		WorkingDir:            taskDir,                              // Other Organiser <workDir>/tasks/<taskID>
		ToolOutputDir:         cmdOutDir(taskDir),
		MaxTurns:              p.maxTurns, // 0 = unlimited (configurable in agent management)
		MaxDuration:           maxDur,     // 0=No limit;Yes deadline hour=Distance deadline Remaining
		Compaction:            compactionConfig(p.compactionWindow()),
		// Plan-to-dos across awakening sharing: keep the chain between multiple rounds(session It's new.,store No).
		Todos: p.todoFor(ts.ID()),
		// hit[Current round]Step budget→ SDK End of the run.:Putting down what we've come up with.(It's a pie. add_intent,
		// I can prove it. prove_goal,Serial chain TodoWrite),Not stop planning.——planner They'll wake up again and again..
		// clamped(Tasked deadline Cracker.)Time to Use PromptByReason(See wrapupSettlementForTask).
		Settlement:   settle,
		NonStreaming: p.nonStreaming(), // The profile Walk when choosing non-stream Provider.Complete
		MaxTokens:    p.maxTokens(),    // 0 = No limit,By the server default
	}
	if p.tx != nil { // persist raw LLM conversation; one accumulating file per task's planner
		opts.Transcript = p.tx
		opts.SessionID = fmt.Sprintf("exp%d-planner", ts.ID())
	}
	// Experimental features:Open by noa Take over context compression(Archive concentrated. <workDir>/noa/<SessionID> Down,Durable).
	noaSession := fmt.Sprintf("exp%d-planner", ts.ID())
	enableNoa(&opts, p.noaEnabledFn, p.workDir, noaSession, noaWarn(noaSession))
	// Trends (intentions just completed) + Now spell the current wheel. user Enter (see below) input).user There's more in there.
	// Command + Awakening To-do(todo It's the model's own plan. user Yeah.).
	// Opening statement press[Any specific changes during current cycle]In two categories: change → Point Down[Actual changes]blocks;no change
	// (Heart beat time check. / hint / Restoring) → Don't lie."Figure changed.",Instead, we're suggesting that we review our running intentions..
	lead := "Review the changes that triggered this round, then plan the next step:"
	if len(triggers) == 0 {
		lead = "This round is a scheduled review without a specific change signal. Review active intents. Use steer_work for stalled or drifting work and kill_work for a fundamentally wrong direction. Assess goals and uncovered directions:"
		// Heartbeat/When no change wakes up,If the whole picture is gone, open or running Intention → The exploration has stopped.(Nope. worker He's running.,
		// There's no line.).Clear announcement planner And force it to take a new course.,Don't just review your intentions and turn around..
		if active, err := ts.HasActiveIntent(); err == nil && !active {
			lead = "This is a scheduled review with no open or running intents. Check whether the goals are met. For every unmet goal, submit at least one new, nonduplicate intent that advances it:"
		}
	}
	input := lead + situational + "\n\nAssess the goals against verified evidence and use prove_goal for each achieved goal. If a goal remains unmet and frontier_open=0 with no running_intents, submit at least one intent that advances it. Submit zero intents only when active work covers the directions or all goals are met." +
		renderPlannerTodos(opts.Todos.List())
	// MaxDuration We'll break the running tools at the end of the clock and finish the ground.(Yes. ctx on),No more single-wheel cards.
	// Around the end.,No external hard work. ctx Bottom.ctx Only carried pause / kill / shutdown.
	_, _, err = captureRun(ctx, opts, input,
		func(r db.Activity) {
			if emit != nil {
				r.Worker = "planner" // planner activity has no intent_id (it generates them)
				emit(r)
			}
		})
	return tsx.GoalMet, tsx.Reason, err
}
