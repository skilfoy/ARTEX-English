// ARTEX domain model — types used across the UI.
// Derived from the functional spec (section 7: Key data shape).

export type TaskStatus = "created" | "queued" | "running" | "paused" | "done" | "failed" | "timeout";
export type EngineMode = "exploring" | "paused" | "stalled" | "idle";

export interface Task {
  id: string;
  name?: string; // Optional task name;Empty/Default=Unnamed,Return to description when displaying
  category_id?: number;
  category_name?: string;
  pinned?: boolean;
  pinned_at?: string | null;
  description: string;
  goal: string;
  status: TaskStatus;
  created_at: string;
  created_unix?: number; // created_at as unix seconds (run-duration calc)
  completed_at?: string; // RFC3339 finish time (done/failed); "" if unfinished
  completed_unix?: number; // completed_at as unix seconds (0/undef if unfinished)
  last_activity_unix?: number; // unix seconds of the last activity (0/undef if none)
  paused?: boolean;
  queued?: boolean;
  active?: boolean;
  in_flight?: number;
  findings?: { critical: number; high: number; medium: number; low: number }; // Number of registered vulnerabilities(Bracked by severity)
  last_activity?: string;
  stalled?: boolean;
  goals_total?: number;
  goals_met?: number;
  engine_mode?: EngineMode;
  tokens?: TokenTotal; // whole-task token consumption
  llm_profile_id?: number; // LLM profile used; absent = default profile
  llm_profile_ids?: number[]; // ordered task-level failover chain
  active_llm_profile_id?: number; // profile used by the next LLM call
  llm_failover_state?: "default" | "ready" | "chain_exhausted" | string;
  llm_failover_reason?: string;
  source_task_ids?: string[]; // directly related tasks inherited as read-only context
  archive_blocked_by_task_id?: string; // live direct dependent that must be archived first
  company_ids?: number[]; // associated company scopes; current company assets join the task at creation
  coverage_enabled?: boolean; // Asset coverage function switch(Creation timing,On by default);false=Not counted/Do not display coverage
}

export interface TaskCategory {
  id: number;
  name: string;
  task_count: number;
  created_at: string;
  updated_at: string;
}

export interface TaskTemplate {
  id: number;
  name: string;
  description: string;
  goal: string;
  category_id?: number | null; // Default classification;null/Default=None
  intercept_rules?: AssetInterceptRuleInput[]; // Preset task-level interception/Allow rules
  created_at: string;
  updated_at: string;
}

export interface DeleteTaskOptions {
  delete_assets: boolean;
  delete_traffic: boolean;
  delete_files: boolean;
  delete_findings: boolean;
  delete_llm_records: boolean;
}

export interface DeleteTaskResult {
  deleted: string;
  assets_deleted: number;
  assets_detached: number;
  traffic_deleted: number;
  files_deleted: boolean;
  findings_deleted: number;
  llm_records_deleted: number;
  cleanup_warning?: string;
}

export type TaskArchiveState =
  | "archive_queued"
  | "archiving"
  | "archive_failed"
  | "ready"
  | "restore_queued"
  | "restoring"
  | "restore_failed"
  | "delete_queued"
  | "deleting"
  | "delete_failed";

export interface TaskArchiveTokenStats {
  calls?: number;
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
}

export interface TaskArchive {
  id: number;
  task_id: number;
  state: TaskArchiveState;
  phase: string;
  progress: number;
  error?: string;
  warnings?: string[];
  format_version: number;
  sha256?: string;
  original_size: number;
  compressed_size: number;
  task_name: string;
  task_description: string;
  task_goal: string;
  original_status: TaskStatus;
  category_id?: number;
  category_name?: string;
  source_task_ids: number[];
  remaining_timeout_seconds: number;
  data_counts: Record<string, number>;
  aggregate_stats: {
    tokens?: TaskArchiveTokenStats;
    skills?: Record<string, number>;
    tools?: Record<string, number>;
    findings?: Record<string, number>;
  };
  archived_at?: string;
  requested_at: string;
  created_at: string;
  updated_at: string;
}

export interface TaskArchivePage {
  items: TaskArchive[];
  total: number;
  page: number;
  size: number;
}

export interface ArchiveBatchItem {
  id: string;
  archive_id?: number;
  ok: boolean;
  queued: boolean;
  error?: string;
}

// ---- Asset graph (global, shared across tasks) ----
export type AssetType =
  | "company"
  | "domain"
  | "ip"
  | "port"
  | "service"
  | "site"
  | "endpoint"
  | "parameter"
  | "tech"
  | "credential"
  | "data";

export type NodeState = "observed" | "confirmed" | "tombstoned";

export interface AssetNode {
  id: string;
  type: AssetType;
  name: string;
  key: string; // nkey
  value?: string;
  company_id?: string; // Attributable company assets id;Empty=Not vested
  state: NodeState;
  confidence: number; // 0..1
  attrs?: Record<string, unknown>;
  first_seen: string;
  last_seen: string;
}

export type AssetRel =
  | "owns"
  | "resolves"
  | "exposes"
  | "runs"
  | "serves"
  | "has_endpoint"
  | "has_param"
  | "fingerprinted"
  | "authenticates_as"
  | "reachable"
  | "has_subdomain";

export interface Edge {
  src: string;
  dst: string;
  rel: AssetRel | ExploreRel;
}

// Task asset view — server-side enriched, paginated.
export interface TaskAssetRef {
  id: string;
  name?: string;
  key: string;
  attrs?: Record<string, unknown>;
}

export interface TaskAssetItem extends AssetNode {
  techs?: TaskAssetRef[];
  auth?: TaskAssetRef[];
  params?: TaskAssetRef[];
}

export interface TaskAssetView {
  counts: Record<string, number>;
  total: number;
  items: TaskAssetItem[];
}

// ---- New unified asset model (new backend) ----
export type NewAssetType = "root_domain" | "ip" | "subdomain" | "app" | "service" | "endpoint";

export interface Asset {
  id: number;
  type: NewAssetType;
  company_id?: number;
  task_ids: number[];
  domain?: string;
  root_domain?: string;
  ip?: string;
  c_segment?: string;
  port?: number;
  icp?: string;
  bound_domains?: string[];
  open_ports?: { port: number; service?: string }[];
  record_type?: string;
  record_value?: string[] | string;
  bundle_id?: string;
  app_name?: string;
  category?: string;
  app_description?: string;
  app_icp?: string;
  url?: string;
  service_type?: string;
  service_name?: string;
  favicon_mmh3?: string;
  status_code?: number;
  content_length?: number;
  page_title?: string;
  technologies?: string[];
  auth?: Record<string, unknown>[];
  method?: string;
  params?: Record<string, unknown>[];
  extra?: Record<string, unknown>;
  last_seen: string;
  task_source?: string;
  task_source_summary?: string;
  task_source_node_id?: number;
}

export interface IntentAsset {
  intent_id: number | string;
  asset_id: number;
  type: NewAssetType;
  label: string;
  source: string;
  source_summary: string;
  source_node_id?: number;
  source_task_id: number;
  inherited: boolean;
}

export interface TaskAssetMutation {
  requested: number;
  attached: number;
  existing: number;
}

export interface TaskAssetScopeMutation {
  requested: number;
  assets_linked: number;
  assets_existing: number;
  scopes_added: number;
  scopes_existing: number;
}

// ---- Asset coverage graph (per task) ----
// Force guidance[Asset coverage map]a node of.key Unique: Asset="a:<id>",Company="c:<id>",
// Root domain name without asset row="r:<domain>".in_scope=false is a gray context node used only for wiring.
export interface CoverageGraphNode {
  key: string;
  kind: "company" | "root_domain" | "subdomain" | "ip" | "service" | "app" | "endpoint";
  label: string;
  tested: boolean;
  in_scope: boolean;
  asset_id?: number;
  company_id?: number;
  domain?: string;
  root_domain?: string;
  ip?: string;
  url?: string;
  port?: number;
  service_type?: string;
  app_name?: string;
  page_title?: string;
  status_code?: number;
}

export interface CoverageGraphEdge {
  src: string;
  dst: string;
}

export interface CoverageGraphData {
  nodes: CoverageGraphNode[];
  edges: CoverageGraphEdge[];
}

// The intent associated with an asset in this task exploration map/fact/Discover (for overlay graph node drawer)).
export interface CoverageAssetRef {
  id: number;
  kind: string;
  state: string;
  summary: string;
  source_task_id?: string;
  inherited?: boolean;
}
export interface CoverageAssetRefs {
  intents: CoverageAssetRef[];
  facts: CoverageAssetRef[];
  findings: CoverageAssetRef[];
}

// ---- Workspace file manager (workDir) ----
export interface WorkspaceEntry {
  name: string;
  path: string; // workspace-relative, forward slashes
  dir: boolean;
  size: number;
  mtime: number; // unix millis
}
export interface WorkspaceListing {
  path: string;
  entries: WorkspaceEntry[];
}
export interface WorkspaceFile {
  path: string;
  size: number;
  binary: boolean;
  too_large?: boolean;
  content?: string;
}

// One item of the task test range (coverage denominator + Authorization Boundary).
export interface TaskScopeRow {
  id: number;
  task_id: number;
  kind: "company" | "root_domain" | "subdomain" | "ip" | "cidr" | "icp" | "keyword";
  company_id?: number;
  company_name?: string; // Backend JOIN companies Parsing, only kind=company Valueful
  domain?: string;
  net?: string;
  value?: string;
  source: "auto" | "agent" | "manual";
  reason?: string;
}

export type CompanyScopeKind = "domain" | "ip" | "cidr" | "icp" | "keyword";

// Structured asset range rules submitted when adding a new enterprise.
export interface CompanyScopeRule {
  kind: CompanyScopeKind;
  value: string;
}

// Results of asset range writing.errors is an illegal line in this submission;warnings is irrelevant to this submission,
// Existing data issues that would make the attribution results not as expected (such as ip The field stores the assets of the host name).
export interface CompanyScopeMutation {
  added: number;
  skipped: number;
  invalid: number;
  errors?: string[];
  warnings?: string[];
}

// One of the company's asset range rules (attributed to the only source of truth).
export interface ScopeRow {
  id: number;
  company_id: number;
  kind: CompanyScopeKind;
  domain?: string; // kind=domain sometimes valuable
  net?: string; // kind=ip|cidr sometimes valuable
  value?: string; // kind=icp|keyword may be returned directly from the backend
  raw: string; // Original user input, used for display and backfill
  reason?: string;
}

// Enterprise:type=company asset node + icon + Asset Count + Asset Scope Rules.
export interface Company {
  id: number;
  name: string;
  logo?: string; // Remote icon URL;If empty, use the first letter of the name on the front end.
  asset_count: number;
  scope?: ScopeRow[];
}

// ---- Exploration graph (per task) ----
export type ExploreKind = "task" | "begin" | "goal" | "intent" | "fact" | "finding" | "hint" | "digest";
export type GoalState = "open" | "met" | "abandoned";
export type IntentState = "open" | "running" | "paused" | "done" | "blocked" | "exhausted" | "stopped";
export type FindingState = "confirmed" | "dismissed";
export type HintState = "active" | "consumed";
export type ExploreRel = "spawns" | "derived_from" | "yields" | "proves" | "covers";

export interface TaskNode {
  id: string;
  type: ExploreKind;
  payload?: string;
  priority: number; // 0..10
  state: string; // GoalState | IntentState | FindingState | HintState
  origin: string;
  ts: string;
  source_task_id?: string;
  inherited?: boolean;
  delete_reason?: string; // Intent to fake delete(state='deleted')Reason for deletion
}

// One page of announcement board:Nodes paginated in order of creation + Edges involved in this page + Node at the other end of the edge(refs,Press id Index),
// So that every broadcast can be explained clearly[Where it comes from and what it produces],Without having to pull down the entire picture.
export interface ExplorationNodePage {
  items: TaskNode[];
  total: number;
  page: number;
  size: number;
  edges: Edge[];
  refs: Record<string, TaskNode>;
  // node id → Assets anchored by this node(Displayed when the announcement board is expanded,Contains the nodes on this page and their neighbors).
  assets: Record<string, FindingAsset[]>;
}

export interface ExplorationNodeQuery {
  page?: number;
  size?: number;
  kinds?: ExploreKind[];
  states?: string[];
  q?: string;
  order?: "asc" | "desc";
}

// Goals for goal management cards(The backend has been payload divided into text/vulnclass).
export interface TaskGoal {
  id: string;
  text: string;
  vulnclass?: string;
  state: string; // GoalState
  origin?: string;
  ts: string;
}

// Operation constraints for constraint management cards(allow=Allow / deny=Prohibited).
export type ConstraintKind = "allow" | "deny";
export interface TaskConstraint {
  id: string;
  kind: ConstraintKind;
  text: string;
  origin?: string;
  ts?: string;
}

// ---- Findings ----
export type Severity = "critical" | "high" | "medium" | "low";

// Vulnerability handling status:Pending / Processing / Confirmed / Processed / Fixed / False positive / Ignore / Repeat / Risk Acceptance.
export type FindingStatus =
  | "pending"
  | "in_progress"
  | "confirmed"
  | "resolved"
  | "fixed"
  | "false_positive"
  | "ignored"
  | "duplicate"
  | "risk_accepted";

// FindingAsset is a vulnerability-bound asset(Already pre-rendered on the backend label).
export interface FindingAsset {
  id: string;
  type: string;
  label: string;
}

export interface Finding {
  traffic_count?: number;
  evidence_version?: number;
  report_evidence_version?: number;
  report_stale?: boolean;
  id: string;
  finding_id?: string; // Independent findings Table rows id,Status update handle(Old nodes in the task may be missing)
  vulnclass: string;
  name?: string; // Vulnerability name;When empty, the display will fall back to vulnclass
  severity: Severity;
  status: FindingStatus;
  summary: string;
  evidence: string;
  report?: string; // Detailed report(Markdown);Only the details interface returns,The list is empty
  intent_id?: string;
  param_id?: string;
  task_id?: string;
  task_description?: string;
  source_task_id?: string;
  inherited?: boolean;
  assets?: FindingAsset[];
  ts: string;
}

// FindingsPage is the server-side paging response of the discovery list.
export interface FindingsPage {
  items: Finding[];
  total: number;
  page: number;
  page_size: number;
}

export interface FindingGroup {
  task_id: string | number | null;
  task_name?: string; // Optional task name;Empty/Default=Unnamed
  task_description: string;
  task_status: string;
  count: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

export interface FindingGroupsPage {
  items: FindingGroup[];
  total: number;
  finding_total: number;
  page: number;
  page_size: number;
}

export interface FindingDeepenResponse {
  task_id: string;
  intent_id: string;
  state: IntentState;
  queued: boolean;
}

// FindingStats is to discover full table aggregation(Statistics Card + Vulnerability type drop-down),Server-side computing,Not affected by paging.
export interface FindingStats {
  total: number;
  pending: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  vulnclasses: string[];
  tasks: FindingTaskOption[];
}

// FindingTaskOption is the discovery page[By task]Filter an item in the drop-down list:Vulnerable tasks(An empty description indicates that the task has been deleted,
// Front-end rollback display id)and its number of vulnerabilities.
export interface FindingTaskOption {
  id: string | number;
  name?: string; // Optional task name;Empty/Default=Unnamed
  description: string;
  count: number;
}

// FindingQuery is the discovery list paging/Filter/Sort parameters.
export interface FindingQuery {
  page: number;
  pageSize: number;
  severity?: "all" | Severity;
  status?: "all" | FindingStatus;
  vulnclass?: string;
  task?: string; // Task id;"all"/Empty = Do not filter by task
  query?: string;
  sort?: "severity" | "time";
  // Asset tree node key;Select a node = Select its entire subtree. empty = Do not filter by assets.
  assetScope?: string;
}

// ---- Findings by asset (Asset view) ----
export type FindingAssetKind = "company" | "root_domain" | "subdomain" | "ip" | "service" | "app" | "endpoint" | "none";

// FindingAssetNode is a node of the asset tree.key Shaped like a:<id>(Assets),c:<id>(Enterprise),
// r:<domain>(There is no root domain name of the asset row in the library),__none__(Unassociated assets).
export interface FindingAssetNode {
  key: string;
  parent?: string;
  kind: FindingAssetKind;
  label: string;
  asset_id?: number;
  company_id?: number;
  self: number; // The number of discoveries directly linked to the asset
  total: number; // Han descendants, remove duplicates as found
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

export interface FindingAssetTree {
  nodes: FindingAssetNode[];
  finding_total: number;
  truncated: boolean;
  dropped_kinds?: string[];
}

// FINDING_UNASSIGNED_ASSET With the backend db.FindingUnassignedAsset Correspond.
export const FINDING_UNASSIGNED_ASSET = "__none__";

// ---- Activity / sessions ----
export type ActivityKind =
  | "tool_use"
  | "tool_result"
  | "text"
  | "thinking"
  | "result"
  | "user"
  | "intent" // LLM-generated exploration objective leading a worker session (UI-synthesized)
  | "round" // planner round boundary marker (engine-emitted)
  | "usage" // live cumulative token usage (per model turn); not rendered
  | "llm_switch" // automatic/manual task-level LLM switch
  | "llm_failover" // task-level provider switch / chain exhaustion audit event
  | "intercept_request"; // user-approval request from the intercept layer

// ChatAttachment It is a file uploaded once:path relative to this session/Task working directory(i.e. agent of CWD).
export interface ChatAttachment {
  name: string;
  path: string;
  size: number;
  abs?: string; // Absolute path(scope=staging Return when uploading temporarily;Write it into the description before creating the task)
}

export interface Activity {
  seq: number;
  intent_id?: string;
  worker: string; // session owner: planner | mainagent | work#1 ...
  ts: string;
  kind: ActivityKind;
  tool?: string;
  tool_use_id?: string;
  is_error?: boolean;
  summary: string;
  detail?: string;
  metadata?: {
    llm_transition?: LLMTransition;
  };
  source_task_id?: string;
  inherited?: boolean;
  main_seg?: number; // main-agent conversation segment (present only on worker="mainagent" rows)
  // token usage (present only on kind='result')
  input_tokens?: number;
  output_tokens?: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
}

export interface LLMAuditProfile {
  id: number;
  name: string;
  format: string;
  model: string;
}

export interface LLMTransition {
  mode: "automatic" | "manual" | "exhausted";
  reason: string;
  previous?: LLMAuditProfile;
  next?: LLMAuditProfile;
}

export interface TaskLLMResolution {
  profile_id?: number;
  name: string;
  format: string;
  model: string;
  source: "task_chain" | "agent_binding" | "global_profile" | "environment" | "global";
  available: boolean;
  reason?: string;
}

export interface TaskLLMResolutions {
  mainagent: TaskLLMResolution;
  planner: TaskLLMResolution;
  worker: TaskLLMResolution;
}

// ---- Agent triggers (P3 Scheduling, custom only agent) ----
export interface AgentTrigger {
  id: number;
  agent_key: string;
  enabled: boolean;
  interval_sec: number; // Timing:each N second(0=Irregular)
  on_finding: boolean; // Any task discovery finding Triggered when
  on_goal_met: boolean; // Triggered when any task reaches its goal
  on_task_timeout: boolean; // Triggered when any task times out
  on_tool_call: boolean; // The selected tool is called(Execution completed)Triggered when
  on_task_create: boolean; // Triggered when any task is created
  interval_message: string; // Independent user messages for each trigger condition
  finding_message: string;
  goal_message: string;
  task_timeout_message: string;
  tool_call_message: string;
  task_create_message: string;
  tool_names: string[]; // on_tool_call Selected tool key(At least one)
  last_fire?: string;
}

// ---- Conversations (chat page) ----
export interface ActiveFindingRetest {
  id: number;
  finding_id: string;
  conversation_id: number;
  status: "pending" | "running";
}

export interface FindingRetest {
  id: number;
  finding_id: number;
  conversation_id: number | null;
  status: "pending" | "running" | "completed" | "failed" | "stopped";
  verdict: "" | "reproduced" | "fixed" | "inconclusive";
  notes: string;
  summary: string;
  evidence: string;
  error: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}

export interface Conversation {
  id: number;
  running?: boolean; // live server state, returned with the conversation list
  agent_key: string;
  title: string;
  llm_profile_id?: number;
  pinned?: boolean;
  pinned_at?: string | null;
  created_at: string;
  updated_at: string;
}

// ---- Backend logs (/logs page) ----
export interface LogLine {
  seq: number;
  db_id?: number; // server_logs.id; present for DB-persisted lines
  ts: string;
  level: "info" | "warn" | "error";
  tag: string;
  text: string;
}

export type SessionRole = "mainagent" | "planner" | "worker" | "system";
export type SessionStatus = "running" | "paused" | "done" | "blocked" | "exhausted" | "pending" | "stopped" | "deleted";

// Daily token aggregate bucket (GET /api/tokens/daily).
export interface DailyTokenBucket {
  date: string; // "YYYY-MM-DD"
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// Per-worker token usage (GET /api/exploration/tokens).
export interface TokenUsage {
  worker: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

export interface SessionTokenUsage {
  session: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

export interface BatchControlItem {
  id: string;
  ok: boolean;
  status?: string;
  queued?: boolean;
  error?: string;
}

// Task-by-task results of batch classification changes. The failure can only be that the task has been deleted, the writing of the category itself is atomic.
export interface BatchCategoryItem {
  id: string;
  ok: boolean;
  error?: string;
}

// Whole-task (all agents) token aggregate.
export interface TokenTotal {
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// Global per-profile token spend from the llm_usage ledger (GET /api/tokens/usage).
export interface ProfileUsage {
  profile_name: string;
  calls: number;
  tasks: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// One (profile, UTC day) token bucket for the dashboard's daily chart (new source).
export interface ProfileDayUsage {
  profile_name: string;
  date: string; // YYYY-MM-DD
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
}

// Response of GET /api/tokens/usage — the dashboard's "new" (llm_usage) token view.
export interface UsageStats {
  by_profile: ProfileUsage[];
  daily: ProfileDayUsage[];
}

// Per-model token usage for one task (GET /api/llm/records/by-model), from the
// always-on llm_usage metering ledger. calls = number of LLM calls on this model.
export interface ModelTokenStat {
  model: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

export interface Session {
  id: string;
  role: SessionRole;
  title: string;
  status: SessionStatus;
  live: boolean;
  last_activity: string;
  intent_id?: string;
  source_task_id?: string;
  inherited?: boolean;
  seg?: number; // main-agent session: which conversation segment (0 = original)
}

// ---- Security ----
export interface AuditEntry {
  ts: string;
  tool: string;
  action: "allow" | "block";
  reason?: string;
  command?: string;
}

export interface Audit {
  entries?: AuditEntry[];
  attributions?: Record<string, number>;
}

// ---- Traffic ----
export interface TrafficExchange {
  id: string;
  ts: string;
  host: string;
  method: string;
  url: string;
  status: number;
  content_type: string;
  resp_len: number;
}

export interface TrafficResp {
  enabled: boolean;
  proxy?: string;
  count?: number; // global total (unfiltered)
  total?: number; // rows matching the current filter (for pagination)
  page?: number;
  size?: number;
  exchanges?: TrafficExchange[];
}

// Full raw request/response of one exchange (lazy-loaded on row select).
export interface TrafficDetail {
  req: string;
  resp: string;
}

// One distinct recorded host with its exchange count (target picker).
export interface TrafficHost {
  host: string;
  count: number;
}

// ---- App settings (runtime toggles) ----
export interface Settings {
  traffic_capture: boolean;
  agent_traffic_binding: boolean; // Agent Automatically bind traffic evidence, turned off by default; does not affect manual binding
  llm_record: boolean; // LLM Recording switch (default off); nothing will be recorded when turned off LLM Call
  // Web search. brave_key_set / tavily_key_set reflect whether a key is stored
  // (the values are never returned). On PUT, send the corresponding field to set/clear.
  web_search_enabled: boolean;
  web_search_backend: string; // "ddgs" | "brave-free" | "tavily" | "deepseek"
  brave_key_set: boolean;
  tavily_key_set: boolean;
  // write-only: only sent on PUT to store/clear the key.
  brave_search_api_key?: string;
  tavily_search_api_key?: string;
  // Independent export agent(http/https/socks5),Used to access the search endpoint; and log traffic MITM Agency is irrelevant. empty=Direct connection.
  web_search_proxy?: string;
  // Global export agent(http/https/socks5,Can be brought user:pass),All targeted traffic goes through it. When traffic capture is turned on as
  // MITM Upstream; inject directly when capture is turned off agent of bash/WebFetch.Empty=Direct connection.
  global_proxy?: string;
  python_interpreter?: string; // Custom script tool python Interpreter path(Empty=Runtime detection)
  workers?: number; // Concurrent work agent Number(Default3);Effective for tasks started later
  // Task concurrency upper limit:At the same time[Running]The maximum number of tasks. closure=No limit;After opening, new tasks will be queued if they exceed the limit.,Automatically start when space is available.
  task_concurrency_enabled?: boolean; // Default false
  task_concurrency_limit?: number; // Default after opening 5
  // LLM Polling(Failover).Off by default; after turning on[No model specified]of agent Not available in current configuration
  // (Insufficient balance/key Invalid/Current limiting/Automatically switch to the next configuration when the service is abnormal).
  llm_pool_enabled?: boolean; // Default false
  // Binded with specified configuration agent/Whether it also falls back to the polling chain when the task fails. Default false = Binding means exclusive.
  llm_pool_bind_fallback?: boolean;
  // Operation constraint injection range(Open by default):Put the task on allow/deny Constraint spelling correspondence agent system prompts.
  constraints_inject_planner?: boolean;
  constraints_inject_worker?: boolean;
  // Experimental features:noa Model driven context compression(Default off).Four types of platform access after opening agent(planner/
  // worker/Lord agent/Dialogue)By noa Take over context compression,Replace built-in compaction;each run Read once,Yes
  // Started after run Effective.
  noa_compaction?: boolean;
  // ---- Vulnerability IM Push (the channel itself is an independent resource, see /api/notify/*,There are only three global configurations here)----
  notify_enabled?: boolean; // Push main switch, on by default; used to stop bleeding with one click during maintenance period
  notify_public_base_url?: string; // External access address of the vulnerability details link; empty=The message is not brought back to the chain
  notify_digest_interval_min?: number; // Summary mode period (minutes), default 30
}

// ---- Vulnerability IM Push ----

// NotificationFilter is the filtering condition of the channel. All fields are optional. The default is no filtering..
// The backend does not verify all fields: press when the configuration is malformed[hit]Process (I would rather push more than miss a push)).
export interface NotificationFilter {
  min_severity?: string; // "" | low | medium | high | critical
  task_ids?: number[]; // Empty=No limit; if not empty, it must overlap with the task to which the vulnerability belongs.
  asset_ids?: number[]; // Empty=No limit; non-empty requires intersection with vulnerability-anchored assets
  vulnclass_include?: string[]; // Empty=Accept all; non-empty requires the vulnerability type to hit any keyword (case-insensitive substring)
  vulnclass_exclude?: string[]; // Hit any keyword to exclude (exclusion takes precedence over inclusion))
  on_status_change?: boolean; // Whether to also receive vulnerability handling status change events
}

// NotificationChannel is a channel instance.config fields vary kind Varies,
// and the credentials field is replaced with "__masked__" Mask value starting with——Reposting it as it is means[No change].
export interface NotificationChannel {
  id: number;
  name: string;
  kind: string;
  enabled: boolean;
  mode: "realtime" | "digest";
  config: Record<string, unknown>;
  filter: NotificationFilter;
  rate_per_min: number;
  created_at: string;
  updated_at: string;
  // secret_keys The back-end is given according to the channel type, and the front-end renders the password box and[Leave blank to make no changes]Tips,
  // Do not hardcode any channel knowledge.
  secret_keys: string[];
}

// NotificationKind Yes /api/notify/meta Returned channel type metadata.
export interface NotificationKind {
  kind: string;
  default_rate_per_min: number;
  secret_keys: string[];
}

export interface NotificationMeta {
  kinds: NotificationKind[];
  enabled: boolean;
  public_base_url: string;
  digest_interval_min: string;
  defaults: { digest_interval_min: number };
  stats: {
    channels: number;
    channels_on: number;
    pending: number;
    failed: number;
    sent_today: number;
    backlog_age_ms: number;
  };
}

// NotificationDelivery is a delivery record, used for delivery history and failed resend.
export interface NotificationDelivery {
  id: number;
  finding_id: string;
  event_kind: string; // finding_created | finding_status_changed
  channel_id: number;
  channel_name: string;
  channel_kind: string;
  state: "pending" | "sending" | "sent" | "failed" | "skipped";
  attempts: number;
  last_error: string;
  batch_id?: number;
  created_at: string;
  sent_at?: string;
  next_attempt_at: string;
  title: string;
  severity: string;
}

// ---- LLM config ----
export interface LLMProfile {
  id: string;
  name: string;
  format: "openai" | "anthropic" | "openai-responses";
  base_url?: string;
  proxy?: string;
  model: string;
  api_key_hint?: string;
  rate_per_second: number;
  rate_per_minute: number;
  context_window_k?: number;
  // Think switch(thinking.type): ""=Do not send(Default) | "disabled"=Close | "enabled"=Open
  thinking_type?: string;
  // Thinking intensity: ""=Do not send(Default) | "low"/"medium"/"high"/"xhigh"/"max"
  reasoning_effort?: string;
  is_default: boolean;
  // Polling order: The larger the value, the first to be selected. The activation configuration is always the head of the chain and has nothing to do with this value..
  priority?: number;
  // true = Not a failover target (can still be agent/Task explicit binding is used).
  pool_exclude?: boolean;
  // true(Default)= Streaming(SSE) | false = True·non-streaming(stream:false,One-time return).
  streaming?: boolean;
  // The output limit of a single reply(token).0 = Do not send this field, determined by the default value of the server.
  // Attention and context_window_k Difference: The latter is the total capacity of the model and is only used locally for compression threshold.
  max_tokens?: number;
  // Which request field name is used for the upper limit, only format="openai" meaningful:
  // ""=max_tokens(Default) | "max_completion_tokens"(OpenAI The inference model only recognizes it)
  max_tokens_field?: string;
  // Customized session header name: When it is not empty, each request will bring this HTTP head, head value=Current session/Intentional session id.
  // ""=Do not send. Used to press session-id Header prompt cache/Gateway for sticky routing.
  session_header_key?: string;
  // This configuration covers retry (connection establishment/Empty response/Same provider safety window). Leave blank/Full 0 = Follow the global strategy.
  retry?: LLMRetryOverride;
}

// ---- LLM Retry strategy ----
// Two knobs for one layer of retries. both[0 = Not configured]:
//   attempts    0=Use default times | -1=Close this layer and try again | >0=Number of retries
//   interval_ms 0=Use default exponential backoff | >0=Use this fixed millisecond interval instead
export interface LLMRetryRule {
  attempts: number;
  interval_ms: number;
}

// Single LLM The three layers that the configuration can cover (all[Follow the endpoint]retry).
export interface LLMRetryOverride {
  connect: LLMRetryRule; // Retry establishing connection: connection reset/Timeout/429/5xx,Before the stream starts
  empty: LLMRetryRule; // Empty response retry: Completed with nothing (only openai Format)
  stream: LLMRetryRule; // Same provider Safe window retry: interruption replay before output is delivered
}

// Global strategy = Default values ​​for the above three layers + The two layers are only global:
//   breaker Polling circuit breaker(attempts=Several consecutive instantaneous fuse failures,interval_ms=Fixed cooling time)
//   intent  Intention to run again(worker With model_error After the end, the whole intention is to run again)
export interface LLMRetryPolicy extends LLMRetryOverride {
  breaker: LLMRetryRule;
  intent: LLMRetryRule;
}

// ---- LLM Polling (failover)----
// A position and health status configured in the polling chain.state:
//   ok       Normal
//   degraded There are consecutive failures but the circuit breaker threshold is not reached
//   tripped  It has been blown and has been skipped during the cooling period.(cooldown_secs is the remaining seconds)
export interface LLMPoolMember {
  profile_id: string;
  name: string;
  model: string;
  format: string;
  priority: number;
  active: boolean; // Whether it is the currently active configuration (always the head of the chain))
  excluded: boolean; // pool_exclude:Do not participate in polling
  state: "ok" | "degraded" | "tripped";
  fails: number;
  trips: number;
  cooldown_secs: number;
  last_error?: string;
  last_at?: string;
}

export interface LLMPoolStatus {
  enabled: boolean;
  bind_fallback: boolean;
  chain: LLMPoolMember[];
}

// ---- Agents ----
export interface Agent {
  id: string;
  key: string; // Built as goals/planner/mainagent/worker;Customized as user-defined key
  name: string;
  description?: string;
  role: string;
  builtin: boolean;
  enabled: boolean;
  llm_profile_id?: number | null; // Bound LLM Configuration;null/absent = Follow the mission/session/Global
  max_turns?: number; // 0 = Unlimited
  run_seconds?: number; // worker The upper limit of a single running wall clock(second);0 = Unlimited
  web_search?: boolean; // Whether to enable network search(Subject to system global switch gate control)
  interactive_shell?: boolean; // Whether to enable interactive shell(Durable PTY Conversation tool family)
  // P3 Post-trigger processing strategy(Customize only agent meaningful)
  trigger_run_mode?: "serial" | "parallel"; // Serial Queuing / Each trigger triggers a concurrent session
  trigger_merge_mode?: "by_task" | "all" | "none"; // Only serial:Merge same tasks / Merge all / Do not merge
  trigger_max_parallel?: number; // Only parallel:each agent Concurrency upper limit;0=No limit
  // Binding quantity(Only list interface returns):Visible MCP / Visible Skill / Binding tools
  mcp_count?: number;
  skill_count?: number;
  tool_count?: number;
}

export interface PromptVar {
  name: string;
  description: string;
  example: string;
  source: "exploration" | "runtime" | "distilled";
}

export interface PromptVersion {
  version: number;
  ts: string;
  note: string;
  template_text: string;
}

export interface AgentDetail {
  agent: Agent;
  prompt: string;
  variables: PromptVar[];
  versions: PromptVersion[];
  visibility: { mcp: number[]; skill: string[] };
  // Bindable LLM Configuration candidates(Supply[Default model]drop down);Current binding see agent.llm_profile_id
  llm_profiles?: { id: number; name: string; model: string; is_default: boolean }[];
  wrapup_prompt?: string; // Saved closing prompt words(Empty=Use built-in default)
  wrapup_default?: string; // Built-in default closing prompt word(Placeholder/Restore default)
  wrapup_max_turns?: number; // Number of closing rounds saved(0=Use built-in default)
  wrapup_max_turns_default?: number; // Built-in default number of closing rounds(Supply "0=DefaultN" Tips)
  // Task-level timeout ending words(Only worker/planner,task_timeout_wrapup_supported=true The partition is displayed only when)
  task_timeout_wrapup_supported?: boolean;
  task_timeout_wrapup_prompt?: string;
  task_timeout_wrapup_default?: string;
  task_timeout_wrapup_max_turns?: number;
  task_timeout_wrapup_max_turns_default?: number;
}

// ---- MCP ----
export interface MCPServer {
  id: number;
  name: string;
  transport: "stdio" | "http" | "sse";
  command?: string;
  args: string[];
  env: Record<string, string>;
  url?: string;
  enabled: boolean;
  insecure?: boolean; // http: skip TLS cert verification (self-signed servers)
  tools?: string[]; // mcp_tools_cache (names only, for the count)
}

export interface MCPTool {
  name: string;
  description: string;
}

// ---- Skills ----
// Fields align with the agentskills.io open specification.
// description covers both "what the skill does" and "when to use it".
export interface SkillItem {
  name: string; // unique key = directory name
  description?: string; // required per spec; covers what + when to use
  license?: string; // optional: SPDX identifier or free text
  compatibility?: string; // optional: environment requirements
  mcps?: string[]; // MCP server names this skill unlocks on load
  files: string[]; // files in the skill directory
  // Call statistics(skill_usage Ledger). never called skill:calls=0,last_used Default.
  calls: number;
  tasks: number; // Number of tasks that have loaded it(chat Sessions do not count)
  usage_agents: string[]; // loaded it agent key
  last_used?: string;
}

// SkillCall It's once Skill() Call (single skill list of recent calls).
export interface SkillCall {
  ts: string;
  agent_key: string;
  task_id: number; // 0 = Non-task scenes (dialogue sessions)
  session_id: string;
  args_len: number;
}

// MissingSkill is named but does not exist skill —— "Want to use it but don't have it"gap.
export interface MissingSkill {
  skill: string;
  calls: number;
  agents: string[];
  last_used?: string;
}

// ---- Tools (Built-in tool directory) ----
// key + handler live in Go; only these fields are page-editable. system tools lock
// the key and the parameter *structure* (name/type/required) — the per-param
// description/default and the agent binding are what move.
export interface Tool {
  key: string;
  system: boolean;
  description: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  schema: Record<string, any>; // full JSON-Schema (object with properties)
  agents: string[]; // bound agent keys
  enabled: boolean;
  kind?: "builtin" | "shell" | "command" | "script" | "http"; // Custom tool type
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  exec?: Record<string, any>; // Custom tool execution specifications(kind!=builtin)
  deferred?: boolean; // schema Delay(SearchExtraTools/ExecuteExtraTool)
  calls?: number; // persistent runtime invocation count (older APIs may omit it)
}

// ---- Stats ----
export interface Stats {
  assets: number;
  engine_mode: EngineMode;
  llm_configured: boolean;
  roe_enabled: boolean;
  findings_confirmed: number;
  active_task?: Partial<Task>;
}

// ---- Intercept Rules ----
export type InterceptAction = "allow" | "deny" | "ask";
export type InterceptMatchTarget = "tool_name" | "tool_input";
export type InterceptMatchType = "string" | "regex";

export interface InterceptRule {
  id: number;
  name: string;
  enabled: boolean;
  priority: number;
  match_target: InterceptMatchTarget;
  match_type: InterceptMatchType;
  pattern: string;
  action: InterceptAction;
  message: string;
  timeout_enabled: boolean;
  timeout_seconds: number;
  timeout_action: "deny" | "allow";
  created_at: string;
  updated_at: string;
}

// ---- Asset Intercept Rules(Asset interception: global blacklist) ----
export type AssetInterceptKind =
  | "exact_domain"
  | "exact_ip"
  | "exact_url"
  | "fuzzy_domain"
  | "fuzzy_ip"
  | "fuzzy_url"
  | "cidr";

// action For task-level rules only:block=Interception(Testing prohibited) allow=Allow(whitelist).
export type AssetInterceptAction = "block" | "allow";

// Task-level asset interception/Allow rule entry items (create tasks, edit task details for use).
export interface AssetInterceptRuleInput {
  action: AssetInterceptAction;
  kind: AssetInterceptKind;
  pattern: string;
  note: string;
  enabled: boolean;
}

export interface AssetInterceptRule {
  id: number;
  enabled: boolean;
  action?: AssetInterceptAction; // Global rules do not have this field (always intercepted); task-level rules are distinguished block/allow
  kind: AssetInterceptKind;
  pattern: string;
  note: string;
  builtin: boolean;
  created_at: string;
  updated_at: string;
}

export interface InterceptPending {
  decision_source?: "rule" | "model" | "unknown" | "";
  id: number;
  rule_id?: number;
  conversation_id?: number;
  task_id?: string;
  agent_name: string;
  tool_name: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  tool_input: Record<string, any>;
  status: "pending" | "allowed" | "denied" | "timeout";
  reason: string; // Rules message Or model judgment reasons(Model Determination Zone [Model] Prefix)
  decided_at?: string;
  created_at: string;
}

// JudgeConfig: Model full approval(Only judged by the model if no interception rules hit)Global configuration of.
export interface JudgeConfig {
  enabled: boolean;
  profile_id: number; // 0 = Follow activation/Default configuration
  prompt: string; // Determination prompt words;GET When not set, the backend backfills the full text of the built-in template
  timeout_seconds: number; // Model call timeout
  fail_action: "allow" | "ask" | "deny"; // Model error/Timeout/Return when unresolvable
  ask_timeout_seconds: number; // Model judgment ask The approval waiting timeout after switching to manual
  ask_timeout_action: "allow" | "deny"; // Default action after approval timeout
}

// JudgeUsage: Model full approval(judge Channel)the accumulation of token Dosage + near N day daily sequence.
export interface JudgeDayUsage {
  date: string; // YYYY-MM-DD (UTC)
  calls: number;
  input_tokens: number;
  output_tokens: number;
}
export interface JudgeUsage {
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  daily: JudgeDayUsage[];
}

export interface InterceptApprovalFilter {
  status?: InterceptPending["status"];
  decision_source?: "rule" | "model" | "unknown";
}

// InterceptApprovalRow enriches InterceptPending with conversation/task and rule context.
export interface InterceptApprovalRow extends InterceptPending {
  conv_title: string; // "" if no linked conversation
  conv_agent_key: string; // "" if no linked conversation
  rule_name: string; // "" if rule was deleted
}

// ── Asset synchronization (ScopeSentry Data source) ──────────────────────────────────────────────
export interface SSProject {
  id: string; // MongoDB ObjectID — used as filter.project
  name: string;
  logo?: string;
  AssetCount?: number;
  tag?: string;
}

export interface SSTask {
  id: string;
  name: string; // used as filter.task
  status?: number;
  progress?: number;
  creatTime?: string;
  endTime?: string;
}

// ConvTokenSummary — one conversation's token total (+ profile/date) for merging
// chat usage into the dashboard token stats. GET /api/tokens/conversations.
export interface ConvTokenSummary {
  llm_profile_id: number | null;
  created_at: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// ---- Command recording (Bash execution history) ----
export interface CommandRecord {
  id: number;
  exploration_id: number;
  worker: string;
  tool: string;
  command: string; // raw tool input (JSON)
  output: string;
  is_error: boolean;
  created_at: string;
}

// Call statistics of a single tool(/commands/stats);errors is the number of failures.
export interface ToolStat {
  tool: string;
  total: number;
  errors: number;
}

// ---- LLM recording ----
export interface LLMRecordItem {
  id: number;
  ts: string;
  model: string;
  profile_name: string;
  session_id: string;
  task_id: string;
  worker: string;
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_write: number;
  status: string;
  error?: string;
}

export interface LLMRecordDetail extends LLMRecordItem {
  request_body: string;
  response_body: string;
  // provider Actually sent and received HTTP Original text: The request is buildBody() Issued in full body(Tools included
  // schema),Response is raw SSE frame. above request_body/response_body is a normalized view,
  // Thrown away tools schema With tool_use Block. Old record is empty.
  raw_request?: string;
  raw_response?: string;
}

// One distinct task with its LLM-record count (task picker on the records page).
export interface LLMTask {
  task_id: string;
  count: number;
}

// The exact JSON sent to the review model, retained for all model verdicts.
export interface InterceptReviewInput {
  version: number;
  background?: {
    // worker_summary is retained only for immutable v2/v3 snapshots.
    source: "user_message" | "worker_summary";
    text: string;
    truncated?: boolean;
  };
  // Version 1 snapshots are immutable and remain readable in historical audits.
  task?: {
    task_id: number;
    description: string;
    goal: string;
    constraints: { id: number; kind: string; text: string; origin: string; created_at: number }[];
    truncated?: boolean;
  };
  working_directory?: string;
  worker_intent?: string;
  turn_input?: string;
  background_truncated?: boolean;
  // Legacy v1/v2 snapshots only; v3 never sends execution history.
  history?: {
    tool_use_id: string;
    tool: string;
    arguments_preview: string;
    result: string;
    status: "succeeded" | "failed";
    truncated?: boolean;
  }[];
  history_truncated?: boolean;
  correlation?: "exact" | "ambiguous" | "unavailable";
  tool_name: string;
  arguments: Record<string, unknown>;
}

// Immutable review snapshot plus separately recorded execution outcome.
export interface InterceptAudit {
  model_input?: InterceptReviewInput;
  model_input_digest?: string;
  run_id?: string;
  tool_use_id?: string;
  correlation: "exact" | "ambiguous" | "unavailable";
  input_digest: string;
  user_message: string;
  user_truncated?: boolean;
  context:
    | { kind: string; tool?: string; tool_use_id?: string; text: string; is_error?: boolean; truncated?: boolean }[]
    | null;
  context_truncated?: boolean;
  captured_at: string;
  model_fallback?: boolean;
  initial_action: "allow" | "ask" | "deny";
  initial_reason: string;
  effective_action?: "allow" | "deny";
  decision_reason?: string;
  rule_name?: string;
  config_digest?: string;
  profile_id?: number;
  execution_status: "not_started" | "not_executed" | "awaiting_result" | "succeeded" | "failed" | "unknown";
  output?: string;
  output_truncated?: boolean;
  execution_ended_at?: string;
}
export interface InterceptDetail extends InterceptApprovalRow {
  audit: InterceptAudit | null;
}

export type TrafficEvidenceRole = "baseline" | "proof" | "verification" | "supporting";
export interface TrafficEvidenceRef {
  traffic_id: string;
  role?: TrafficEvidenceRole;
  note?: string;
}
export interface TrafficEvidenceSnapshot {
  id: string;
  source_traffic_id: string;
  captured_at: number;
  url: string;
  method: string;
  status: number;
  content_type: string;
  req_head?: string;
  resp_head?: string;
  req_hash: string;
  resp_hash: string;
  req_len: number;
  resp_len: number;
}
export interface FindingTrafficBinding {
  id: string;
  finding_id: string;
  snapshot_id: string;
  role: TrafficEvidenceRole;
  note: string;
  position: number;
  created_at: string;
  snapshot: TrafficEvidenceSnapshot;
}
export interface FindingTraffic {
  finding_id: string;
  version: number;
  report_version: number;
  bindings: FindingTrafficBinding[];
}
export interface EvidenceBodyPreview {
  content: string;
  offset: number;
  total: number;
  next_offset: number;
  truncated: boolean;
  binary: boolean;
}
export interface FindingTrafficDetail {
  binding: FindingTrafficBinding;
  request: EvidenceBodyPreview;
  response: EvidenceBodyPreview;
}

/** GET /api/update/check —— Current version with GitHub Comparison results of the latest official version. */
export interface UpdateCheck {
  /** Currently running version; development build is "dev" or git describe The suffixed form of. */
  current: string;
  /** Operating state.docker Downloading and reinstalling only affects the writable layer of the container. Rebuilding the container will return the image version.. */
  mode: "docker" | "binary";
  os: string;
  arch: string;
  repo: string;
  /** Is there a previous version that can be rolled back?(artex.old). */
  has_backup: boolean;
  /** Conclusion of self-update bootstrapping during this startup (replacement failed / Rolled back, etc.), empty when nothing happens. */
  boot_notice?: string;
  rolled_back?: boolean;
  /** Query GitHub Give the reason when it fails. At this time, the following fields will not be available.. */
  error?: string;
  latest?: string;
  notes?: string;
  html_url?: string;
  published_at?: string;
  /** The release package name corresponding to the current platform, and the Release Did you really bring it?. */
  asset?: string;
  asset_available?: boolean;
  size?: number;
  has_update?: boolean;
  /** Whether the version numbers of both parties are comparable; the development build is false,Disable one-click updates at this time. */
  comparable?: boolean;
  /** comparable for false Description of the time. */
  reason?: string;
}

/** /api/update/stream The progress of an update pushed. */
export interface UpdateProgress {
  phase: "idle" | "downloading" | "verifying" | "extracting" | "staged" | "failed";
  /** Only the download phase is meaningful(0-100);The remaining stages are -1. */
  percent: number;
  message: string;
  version?: string;
  error?: string;
}

// Original execution selected from an approval, never submitted to the reviewer.
export interface InterceptExecution {
  conversation_id: number | null;
  task_id: string | null;
  session: string;
  seq: number;
  items: Activity[];
}
