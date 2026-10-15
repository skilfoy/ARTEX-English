// MOCK_GET_ROUTES is the verified GET route list for the demo UI.
// An unmatched GET throws only when MOCK_STRICT=1. Otherwise the handler keeps the safe [] / {} fallback.
// POST, PUT, PATCH, and DELETE still end at { ok: true } when no branch matches.
// Routes run only when NEXT_PUBLIC_MOCK=1, through http() in api.ts.

import {
  classifyCompanyScopeLine,
  companyScopeRuleError,
  isCompanyScopeKind,
  normalizeCompanyScopeValue,
} from "../company-scope";
import type {
  Activity,
  ArchiveBatchItem,
  Asset,
  BatchControlItem,
  Company,
  CompanyScopeRule,
  Conversation,
  EvidenceBodyPreview,
  FindingRetest,
  FindingTraffic,
  FindingTrafficBinding,
  IntentAsset,
  ScopeRow,
  Task,
  TaskArchive,
  TaskAssetMutation,
  TaskAssetScopeMutation,
  TaskCategory,
  TaskLLMResolution,
  TaskScopeRow,
  TaskTemplate,
  TrafficEvidenceRole,
  TrafficEvidenceSnapshot,
} from "../types";
import * as D from "./data";

const delay = (ms = 120) => new Promise((r) => setTimeout(r, ms));

// Requests mutate a runtime copy, never the exported fixtures. This keeps module
// initialization deterministic for tests/HMR while preserving state across mock calls.
const mockInterceptHistory = structuredClone(D.interceptHistory);
const mockInterceptPending = structuredClone(D.interceptPending);
const mockInterceptDetails = structuredClone(D.interceptDetails);
const mockTasks = structuredClone(D.tasks);
const mockFindings = structuredClone(D.findings);
const mockLLMRecords = structuredClone(D.llmRecords);
const mockTaskTemplates = structuredClone(D.taskTemplates);
const mockTaskCategories = structuredClone(D.taskCategories);
const mockConversations = structuredClone(D.conversations);
const mockRetests: FindingRetest[] = D.findingRetestSeeds.map((seed, index) => {
  const findingIndex = D.findings.findIndex((item) => item.id === seed.finding_id);
  if (findingIndex < 0) throw new Error(`Demo retest is missing finding ${seed.finding_id}`);
  const when = D.findings[findingIndex]?.ts ?? new Date(0).toISOString();
  return {
    id: index + 1,
    finding_id: findingIndex + 1,
    conversation_id: null,
    status: "completed",
    verdict: seed.verdict,
    notes: "",
    summary: seed.summary,
    evidence: "This preview did not send a request.",
    error: "",
    created_at: when,
    started_at: when,
    finished_at: when,
  };
});
const mockRetestMessages: Record<number, Activity[]> = {};

// Associated traffic for a finding. Every finding has at least one recorded exchange.
// Request and response text comes from trafficBodies, one body per exchange id.
const mockExchangeById = new Map((D.traffic.exchanges ?? []).map((exchange) => [exchange.id, exchange]));

interface MockBindingSeed {
  traffic_id: string;
  role: TrafficEvidenceRole;
  note?: string;
}

// Runtime status:finding id → Binding list(Can add, delete or change the order,demo Memory state).First access initialized by seed.
const mockFindingTraffic: Record<string, FindingTrafficBinding[]> = {};
const mockFindingTrafficVersion: Record<string, number> = {};
let mockBindingSeq = 900;

function mockBuildSnapshot(trafficId: string): TrafficEvidenceSnapshot {
  const exchange = mockExchangeById.get(trafficId);
  const capturedAt = exchange ? Math.floor(Date.parse(exchange.ts) / 1000) : 0;
  let pathAndQuery = "/";
  try {
    if (exchange) pathAndQuery = new URL(exchange.url).pathname + new URL(exchange.url).search;
  } catch {
    // Use the root path as a guarantee.
  }
  const body = D.trafficBodies[trafficId] ?? { req: "", resp: "" };
  return {
    id: `snap-${trafficId}`,
    source_traffic_id: trafficId,
    captured_at: capturedAt,
    url: exchange?.url ?? "",
    method: exchange?.method ?? "GET",
    status: exchange?.status ?? 0,
    content_type: exchange?.content_type ?? "",
    req_head: `${exchange?.method ?? "GET"} ${pathAndQuery} HTTP/1.1\nHost: ${exchange?.host ?? ""}`,
    resp_head: `HTTP/1.1 ${exchange?.status ?? 0}\nContent-Type: ${exchange?.content_type ?? ""}`,
    req_hash: `req-${trafficId}`,
    resp_hash: `resp-${trafficId}`,
    req_len: body ? new TextEncoder().encode(body.req).length : 0,
    resp_len: exchange?.resp_len ?? (body ? new TextEncoder().encode(body.resp).length : 0),
  };
}

function mockTrafficBindings(findingID: string): FindingTrafficBinding[] {
  if (!mockFindingTraffic[findingID]) {
    const seeds = D.findingTrafficSeeds[findingID] ?? [];
    mockFindingTraffic[findingID] = seeds.map((seed, index) => ({
      id: `b-${findingID}-${index + 1}`,
      finding_id: findingID,
      snapshot_id: `snap-${seed.traffic_id}`,
      role: seed.role,
      note: seed.note ?? "",
      position: index,
      created_at: new Date(Date.now() - (seeds.length - index) * 1000).toISOString(),
      snapshot: mockBuildSnapshot(seed.traffic_id),
    }));
    mockFindingTrafficVersion[findingID] = 1;
  }
  return mockFindingTraffic[findingID];
}

function mockTrafficSummary(findingID: string): FindingTraffic {
  const bindings = mockTrafficBindings(findingID).map((binding, index) => ({
    ...binding,
    position: index,
    // List/Digest interface strips off message headers,With the backend trafficSummary Consistent.
    snapshot: { ...binding.snapshot, req_head: "", resp_head: "" },
  }));
  const version = mockFindingTrafficVersion[findingID] ?? 1;
  return { finding_id: findingID, version, report_version: version, bindings };
}

function mockTrafficCount(findingID: string): number {
  return mockTrafficBindings(findingID).length;
}

function mockEvidencePreview(text: string): EvidenceBodyPreview {
  return { content: text, offset: 0, total: text.length, next_offset: text.length, truncated: false, binary: false };
}

function advanceMockRetests() {
  for (const retest of mockRetests) {
    if (retest.status !== "running" || retest.conversation_id == null) continue;
    if (Date.now() - Date.parse(retest.created_at) < 15000) continue;
    retest.status = "completed";
    retest.verdict = "inconclusive";
    retest.summary = "The demo environment did not perform real verification and the current status of the vulnerability cannot be confirmed.";
    retest.evidence =
      "### Demo record\n\nThe original vulnerability has been associated. This environment is not connected to the real Agent, nor is it sending requests to the target; please perform retesting in an actual deployment.";
    retest.finished_at = new Date().toISOString();
    mockRetestMessages[retest.conversation_id].push({
      seq: 3,
      worker: "retester",
      ts: retest.finished_at,
      kind: "text",
      summary: retest.summary,
      detail: retest.evidence,
    });
  }
}

function stopMockRetest(conversationID: number) {
  for (const retest of mockRetests) {
    if (retest.conversation_id !== conversationID || !["pending", "running"].includes(retest.status)) continue;
    retest.status = "stopped";
    retest.finished_at = new Date().toISOString();
    retest.error = "Demo retest has been stopped";
  }
}
const mockIntents = structuredClone(D.intents);
const mockCompanies = structuredClone(D.companies);
const mockAssets = structuredClone(D.assets);
const mockActivity = structuredClone(D.activity);
type MockTaskArchiveSnapshot = {
  task: Task;
  numericTaskID: number;
  assetIDs: number[];
  assetSources: Array<[number, MockTaskAssetSource]>;
};
type MockTaskArchive = TaskArchive & { snapshot: MockTaskArchiveSnapshot };

// Finished demo tasks keep a ready archive so the archive list is not a single row.
// The live task list is left in place so findings still resolve.
function seedDemoTaskArchive(taskId: string, archiveId: number): MockTaskArchive {
  const source = D.tasks.find((item) => item.id === taskId);
  if (!source) throw new Error(`Demo archive task is missing: ${taskId}`);
  const task = structuredClone(source);
  const numericTaskID = D.tasks.findIndex((item) => item.id === task.id) + 1;
  const when = task.completed_at ?? task.created_at;
  const assetIDs = D.assets.filter((asset) => asset.task_ids.includes(numericTaskID)).map((asset) => asset.id);
  const findingCount = D.findings.filter((finding) => finding.task_id === task.id).length;
  return {
    id: archiveId,
    task_id: numericTaskID,
    state: "ready",
    phase: "Archiving completed",
    progress: 100,
    format_version: 1,
    original_size: 4096 + assetIDs.length * 256,
    compressed_size: 1300 + assetIDs.length * 64,
    task_name: task.name ?? "",
    task_description: task.description,
    task_goal: task.goal,
    original_status: task.status,
    category_id: task.category_id,
    category_name: task.category_name,
    source_task_ids: [],
    remaining_timeout_seconds: 0,
    data_counts: { assets: assetIDs.length, findings: findingCount, llm_records: 0 },
    aggregate_stats: {
      tokens: {
        input_tokens: task.tokens?.input_tokens ?? 0,
        output_tokens: task.tokens?.output_tokens ?? 0,
        cache_read_tokens: task.tokens?.cache_read_tokens ?? 0,
        cache_write_tokens: task.tokens?.cache_write_tokens ?? 0,
      },
      findings: { ...(task.findings ?? {}) },
    },
    archived_at: when,
    requested_at: when,
    created_at: task.created_at,
    updated_at: when,
    snapshot: { task, numericTaskID, assetIDs, assetSources: [] },
  };
}

const mockTaskArchives: MockTaskArchive[] = [seedDemoTaskArchive("t-vpn-edge", 1), seedDemoTaskArchive("t-northwind", 2)];
let nextMockTaskArchiveID = 3;
const mockTaskAssetIDs = new Map(D.tasks.map((task, index) => [task.id, index + 1]));
const mockTaskScopes = new Map<string, TaskScopeRow[]>();
type MockTaskAssetSource = Pick<Asset, "task_source" | "task_source_summary" | "task_source_node_id">;
const mockTaskAssetSources = new Map<string, MockTaskAssetSource>();
let nextMockTaskAssetID = D.tasks.length + 1;
let mockActiveTask = D.ACTIVE_TASK;

function mockTaskAssetSourceKey(taskID: string, assetID: number): string {
  return JSON.stringify([taskID, assetID]);
}

function publicMockTaskArchive(archive: MockTaskArchive): TaskArchive {
  const { snapshot: _snapshot, ...item } = archive;
  return structuredClone(item);
}

function mockArchiveTaskID(taskID: string): number {
  const numeric = Number(taskID);
  if (Number.isSafeInteger(numeric) && numeric > 0) return numeric;
  return mockTaskAssetID(taskID) ?? 0;
}

function mockTaskArchiveBlocker(taskID: string): string | undefined {
  return mockTasks.find(
    (candidate) =>
      candidate.id !== taskID &&
      (candidate.source_task_ids ?? []).map(String).includes(taskID) &&
      !mockTaskArchives.some(
        (archive) =>
          archive.snapshot.task.id === candidate.id &&
          (archive.state === "archive_queued" || archive.state === "archiving"),
      ),
  )?.id;
}

function publicMockTask(task: Task): Task {
  const item = structuredClone(task);
  const blocker = mockTaskArchiveBlocker(task.id);
  if (blocker) item.archive_blocked_by_task_id = blocker;
  else delete item.archive_blocked_by_task_id;
  return item;
}

function mockArchiveTask(taskID: string): MockTaskArchive {
  const task = mockTasks.find((item) => item.id === taskID);
  if (!task) throw new Error("Task does not exist");
  if (!["paused", "done", "failed", "timeout"].includes(task.status) && !task.paused) {
    throw new Error(task.queued ? "Pause this queued task first" : "Pause running tasks first");
  }
  const existing = mockTaskArchives.find((item) => item.task_id === mockArchiveTaskID(taskID));
  if (existing) throw new Error("The task is already in the archive queue");
  const dependent = mockTasks.find(
    (candidate) =>
      candidate.id !== taskID &&
      (candidate.source_task_ids ?? []).map(String).includes(taskID) &&
      !mockTaskArchives.some(
        (archive) =>
          archive.snapshot.task.id === candidate.id &&
          (archive.state === "archive_queued" || archive.state === "archiving"),
      ),
  );
  if (dependent) throw new Error(`Task #${dependent.id} still depends on this task, so it cannot be archived yet`);

  const numericTaskID = mockArchiveTaskID(taskID);
  const assetIDs = mockAssets.filter((asset) => asset.task_ids.includes(numericTaskID)).map((asset) => asset.id);
  const assetSources: Array<[number, MockTaskAssetSource]> = [];
  for (const assetID of assetIDs) {
    const source = mockTaskAssetSources.get(mockTaskAssetSourceKey(taskID, assetID));
    if (source) assetSources.push([assetID, structuredClone(source)]);
  }
  const findings = mockFindings.filter((finding) => finding.task_id === taskID);
  const llmRecords = mockLLMRecords.filter((record) => record.task_id === taskID);
  const tokens = task.tokens ?? { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0 };
  const now = new Date().toISOString();
  const archive: MockTaskArchive = {
    id: nextMockTaskArchiveID++,
    task_id: numericTaskID,
    state: "archive_queued",
    phase: "Waiting for archiving",
    progress: 0,
    format_version: 1,
    original_size: 0,
    compressed_size: 0,
    task_name: task.name ?? "",
    task_description: task.description,
    task_goal: task.goal,
    original_status: task.status,
    category_id: task.category_id,
    category_name: task.category_name,
    source_task_ids: (task.source_task_ids ?? []).map(mockArchiveTaskID).filter((id) => id > 0),
    remaining_timeout_seconds: 0,
    data_counts: {
      assets: assetIDs.length,
      findings: findings.length,
      llm_records: llmRecords.length,
    },
    aggregate_stats: {
      tokens: {
        calls: llmRecords.length,
        input_tokens: tokens.input_tokens,
        output_tokens: tokens.output_tokens,
        cache_read_tokens: tokens.cache_read_tokens,
        cache_write_tokens: tokens.cache_write_tokens,
      },
      findings: findings.reduce<Record<string, number>>((counts, finding) => {
        counts[finding.severity] = (counts[finding.severity] ?? 0) + 1;
        return counts;
      }, {}),
    },
    requested_at: now,
    created_at: now,
    updated_at: now,
    snapshot: { task: structuredClone(task), numericTaskID, assetIDs, assetSources },
  };
  mockTaskArchives.unshift(archive);
  setTimeout(() => {
    if (archive.state !== "archive_queued") return;
    archive.state = "archiving";
    archive.phase = "Compress task data";
    archive.progress = 55;
    archive.updated_at = new Date().toISOString();
  }, 100);
  setTimeout(() => {
    if (archive.state !== "archiving" && archive.state !== "archive_queued") return;
    archive.state = "ready";
    archive.phase = "Archiving completed";
    archive.progress = 100;
    archive.archived_at = new Date().toISOString();
    archive.updated_at = archive.archived_at;
    archive.original_size = Math.max(4096, JSON.stringify(archive.snapshot).length * 4);
    archive.compressed_size = Math.max(1024, Math.round(archive.original_size * 0.32));
    const index = mockTasks.findIndex((item) => item.id === taskID);
    if (index >= 0) mockTasks.splice(index, 1);
    for (const asset of mockAssets) asset.task_ids = asset.task_ids.filter((id) => id !== numericTaskID);
    deleteMockTaskAssetSources(taskID);
    if (mockActiveTask === taskID) {
      mockActiveTask = mockTasks[0]?.id ?? "";
      for (const item of mockTasks) item.active = item.id === mockActiveTask;
    }
  }, 500);
  return archive;
}

function mockRestoreArchive(archive: MockTaskArchive): void {
  if (archive.state !== "ready" && archive.state !== "restore_failed") throw new Error("The current archive status cannot be restored");
  archive.state = "restore_queued";
  archive.phase = "Waiting for restore";
  archive.progress = 0;
  archive.error = undefined;
  archive.updated_at = new Date().toISOString();
  setTimeout(() => {
    if (archive.state !== "restore_queued") return;
    archive.state = "restoring";
    archive.phase = "Restore task data";
    archive.progress = 60;
    archive.updated_at = new Date().toISOString();
  }, 100);
  setTimeout(() => {
    if (archive.state !== "restoring" && archive.state !== "restore_queued") return;
    const restored = structuredClone(archive.snapshot.task);
    restored.active = false;
    restored.queued = false;
    if (!mockTasks.some((task) => task.id === restored.id)) mockTasks.push(restored);
    mockTaskAssetIDs.set(restored.id, archive.snapshot.numericTaskID);
    for (const assetID of archive.snapshot.assetIDs) {
      const asset = mockAssets.find((candidate) => candidate.id === assetID);
      if (asset && !asset.task_ids.includes(archive.snapshot.numericTaskID)) {
        asset.task_ids.push(archive.snapshot.numericTaskID);
      }
    }
    for (const [assetID, source] of archive.snapshot.assetSources) setMockTaskAssetSource(restored.id, assetID, source);
    const index = mockTaskArchives.indexOf(archive);
    if (index >= 0) mockTaskArchives.splice(index, 1);
    sortMockTasks();
  }, 500);
}

function mockDeleteArchive(archive: MockTaskArchive): void {
  if (archive.state !== "ready" && archive.state !== "delete_failed") throw new Error("The current archive status cannot be permanently deleted");
  const dependent = mockTaskArchives.find(
    (candidate) => candidate.id !== archive.id && candidate.source_task_ids.includes(archive.task_id),
  );
  if (dependent) throw new Error(`Archive #${dependent.task_id} still depends on this archive, so it cannot be permanently deleted`);
  archive.state = "delete_queued";
  archive.phase = "Waiting for permanent deletion";
  archive.progress = 0;
  archive.error = undefined;
  archive.updated_at = new Date().toISOString();
  setTimeout(() => {
    if (archive.state !== "delete_queued") return;
    archive.state = "deleting";
    archive.phase = "Delete archive package";
    archive.progress = 70;
  }, 100);
  setTimeout(() => {
    const index = mockTaskArchives.indexOf(archive);
    if (index >= 0) mockTaskArchives.splice(index, 1);
  }, 450);
}

function setMockTaskAssetSource(taskID: string, assetID: number, source: MockTaskAssetSource) {
  mockTaskAssetSources.set(mockTaskAssetSourceKey(taskID, assetID), source);
}

function mockAssetForTask(taskID: string, asset: Asset): Asset {
  const source = mockTaskAssetSources.get(mockTaskAssetSourceKey(taskID, asset.id));
  return source ? { ...asset, ...source } : asset;
}

function deleteMockTaskAssetSources(taskID: string, assetID?: number) {
  if (assetID !== undefined) {
    mockTaskAssetSources.delete(mockTaskAssetSourceKey(taskID, assetID));
    return;
  }
  for (const key of mockTaskAssetSources.keys()) {
    const [linkedTaskID] = JSON.parse(key) as [string, number];
    if (linkedTaskID === taskID) mockTaskAssetSources.delete(key);
  }
}

function mockTaskAssetID(taskID: string): number | undefined {
  const numeric = Number(taskID);
  if (Number.isInteger(numeric) && numeric > 0) return numeric;
  return mockTaskAssetIDs.get(taskID);
}

function mockAssetCounts(taskID?: string | null): Record<string, number> {
  const numericTaskID = taskID ? mockTaskAssetID(taskID) : undefined;
  return mockAssets.reduce<Record<string, number>>((counts, asset) => {
    if (taskID && (numericTaskID === undefined || !asset.task_ids.includes(numericTaskID))) return counts;
    counts[asset.type] = (counts[asset.type] ?? 0) + 1;
    return counts;
  }, {});
}

function mockAssetMatchesDSL(asset: Asset, dsl: string): boolean {
  const query = dsl
    .replaceAll(/[()"]/g, " ")
    .replaceAll(/\b(?:AND|OR)\b/gi, " ")
    .replaceAll(/\b[a-z_][a-z0-9_]*(?:==|!=|>=|<=|=|>|<)/gi, " ")
    .trim()
    .toLowerCase();
  if (!query) return true;
  const haystack = JSON.stringify(asset).toLowerCase();
  return query.split(/\s+/).every((term) => haystack.includes(term));
}

// mockFilterFindings Apply public filtering for discovery pages(Severity/Status/Type/Task/Keywords),Assets
// Subtree filtering is done separately mockApplyAssetScope —— With the backend FindingFilter.where() The division of labor is consistent.
function mockFilterFindings(q: URLSearchParams): (typeof mockFindings)[number][] {
  let list = mockFindings.filter((finding) => mockFindingMatchesQuery(finding, q.get("q")));
  const severity = q.get("severity");
  const status = q.get("status");
  const vulnclass = q.get("vulnclass");
  const taskID = q.get("task_id");
  if (severity) list = list.filter((finding) => finding.severity === severity);
  if (status) list = list.filter((finding) => finding.status === status);
  if (vulnclass) list = list.filter((finding) => finding.vulnclass === vulnclass);
  if (taskID === "__unassigned__") list = list.filter((finding) => !finding.task_id);
  else if (taskID) list = list.filter((finding) => finding.task_id === taskID);
  return list;
}

function mockFindingMatchesQuery(finding: (typeof mockFindings)[number], query: string | null): boolean {
  const needle = query?.trim().toLowerCase();
  if (!needle) return true;
  return [finding.name, finding.vulnclass, finding.summary, finding.evidence, finding.report].some((value) =>
    String(value ?? "")
      .toLowerCase()
      .includes(needle),
  );
}

// ── [By assets]View ────────────────────────────────────────────────────────────
// The backend builds the tree in db/finding_assets.go inside(Only collect discovered assets + Completing the ancestors layer by layer,Counting edge
// Ancestral chain deduplication and accumulation).Replay it again in memory using the same set of parent-child priorities.,Let demo Levels of patterns,
// Counting and subtree filtering are consistent with the true backend.

const UNASSIGNED_ASSET = "__none__";

function mockAssetLabel(asset: (typeof mockAssets)[number]): string {
  // No URL service port,Otherwise the tag will be different from the host IP/The domain name line is exactly the same(Consistent with backend).
  if (asset.type === "service" && !asset.url) {
    const host = asset.domain || asset.ip;
    if (host && asset.port) return `${host}:${asset.port}`;
  }
  return asset.url || asset.domain || asset.ip || asset.app_name || `#${asset.id}`;
}

// mockAssetHost With the backend hostPortOf Consistent:Priority domain,Secondly URL inside host,Finally ip.
function mockAssetHost(asset: (typeof mockAssets)[number]): { host: string; port: number } {
  let host = asset.domain ?? "";
  let port = asset.port ?? 0;
  if (!host && asset.url) {
    try {
      const url = new URL(asset.url);
      host = url.hostname.replace(/^\[|\]$/g, "");
      if (!port) port = Number(url.port) || (url.protocol === "https:" ? 443 : 80);
    } catch {
      // Illegal URL Just return it ip.
    }
  }
  if (!host) host = asset.ip ?? "";
  return { host, port };
}

interface MockAssetTreeNode {
  key: string;
  parent?: string;
  kind: string;
  label: string;
  asset_id?: number;
  company_id?: number;
  self: number;
  total: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

// mockBuildAssetTree from a batch(Already filtered by other conditions)Discover building asset tree.
function mockBuildAssetTree(list: (typeof mockFindings)[number][]): MockAssetTreeNode[] {
  const hit = new Set<number>();
  for (const finding of list) {
    for (const ref of finding.assets ?? []) hit.add(Number(ref.id));
  }

  // Collect hit assets + Completing the ancestors layer by layer(Host service / Subdomain name / IP / Root domain name).
  const picked = new Map<number, (typeof mockAssets)[number]>();
  for (const asset of mockAssets) if (hit.has(asset.id)) picked.set(asset.id, asset);
  for (let round = 0; round < 4; round++) {
    const before = picked.size;
    for (const asset of [...picked.values()]) {
      const { host } = mockAssetHost(asset);
      const wanted = mockAssets.filter((candidate) => {
        if (picked.has(candidate.id)) return false;
        if (asset.type === "endpoint" && candidate.type === "service") {
          return mockAssetHost(candidate).host === host;
        }
        if (asset.type === "service" || asset.type === "endpoint") {
          return (
            (candidate.type === "subdomain" && candidate.domain === host) ||
            (candidate.type === "ip" && candidate.ip === host)
          );
        }
        if (asset.type === "subdomain") {
          return candidate.type === "root_domain" && candidate.domain === asset.root_domain;
        }
        return false;
      });
      for (const candidate of wanted) picked.set(candidate.id, candidate);
    }
    if (picked.size === before) break;
  }

  const nodes = new Map<string, MockAssetTreeNode>();
  const parentOf = new Map<string, string>();
  const key = (id: number) => `a:${id}`;
  for (const asset of picked.values()) {
    nodes.set(key(asset.id), {
      key: key(asset.id),
      kind: asset.type,
      label: mockAssetLabel(asset),
      asset_id: asset.id,
      company_id: asset.company_id,
      self: 0,
      total: 0,
      critical: 0,
      high: 0,
      medium: 0,
      low: 0,
      last_found_at: "",
    });
  }

  // Father-son relationship:With db/finding_assets.go of firstOf The priority order is consistent.
  const find = (predicate: (a: (typeof mockAssets)[number]) => boolean) => {
    const asset = [...picked.values()].find(predicate);
    return asset ? key(asset.id) : "";
  };
  for (const asset of picked.values()) {
    const { host, port } = mockAssetHost(asset);
    let parent = "";
    if (asset.type === "subdomain") {
      parent = find((a) => a.type === "root_domain" && a.domain === asset.root_domain);
    } else if (asset.type === "service") {
      parent =
        find((a) => a.type === "subdomain" && (a.domain === asset.domain || a.domain === host)) ||
        find((a) => a.type === "ip" && (a.ip === asset.ip || a.ip === host)) ||
        find((a) => a.type === "root_domain" && a.domain === asset.root_domain);
    } else if (asset.type === "endpoint") {
      parent =
        find((a) => {
          if (a.type !== "service") return false;
          const svc = mockAssetHost(a);
          return svc.host === host && svc.port === port;
        }) ||
        find((a) => a.type === "service" && mockAssetHost(a).host === host) ||
        find((a) => a.type === "subdomain" && (a.domain === host || a.domain === asset.domain)) ||
        find((a) => a.type === "ip" && (a.ip === host || a.ip === asset.ip)) ||
        find((a) => a.type === "root_domain" && a.domain === asset.root_domain);
    }
    if (parent && parent !== key(asset.id)) {
      parentOf.set(key(asset.id), parent);
      const node = nodes.get(key(asset.id));
      if (node) node.parent = parent;
    }
  }

  // Enterprise level:Only give top-level assets that are truly vested(Root domain name / IP / Application)Supplement,If you don't belong, you are at the top.
  for (const node of [...nodes.values()]) {
    if (node.parent || !node.company_id) continue;
    if (!["root_domain", "ip", "app"].includes(node.kind)) continue;
    const company = mockCompanies.find((candidate) => candidate.id === node.company_id);
    if (!company) continue;
    const companyKey = `c:${company.id}`;
    if (!nodes.has(companyKey)) {
      nodes.set(companyKey, {
        key: companyKey,
        kind: "company",
        label: company.name,
        company_id: company.id,
        self: 0,
        total: 0,
        critical: 0,
        high: 0,
        medium: 0,
        low: 0,
        last_found_at: "",
      });
    }
    node.parent = companyKey;
    parentOf.set(node.key, companyKey);
  }

  // count:A discovery goes up the ancestor chain of each of its assets,Collect after deduplication key Gather together and then one by one +1.
  const unassigned: MockAssetTreeNode = {
    key: UNASSIGNED_ASSET,
    kind: "none",
    label: "Unassociated assets",
    self: 0,
    total: 0,
    critical: 0,
    high: 0,
    medium: 0,
    low: 0,
    last_found_at: "",
  };
  const bump = (node: MockAssetTreeNode, finding: (typeof mockFindings)[number]) => {
    node.total++;
    if (finding.severity === "critical") node.critical++;
    else if (finding.severity === "high") node.high++;
    else if (finding.severity === "medium") node.medium++;
    else if (finding.severity === "low") node.low++;
    if (finding.ts > node.last_found_at) node.last_found_at = finding.ts;
  };
  for (const finding of list) {
    const direct = (finding.assets ?? []).map((ref) => nodes.get(`a:${ref.id}`)).filter((n) => n !== undefined);
    if (direct.length === 0) {
      bump(unassigned, finding);
      unassigned.self++;
      continue;
    }
    const touched = new Set<string>();
    for (const node of direct) {
      node.self++;
      for (let cur: string | undefined = node.key; cur; cur = parentOf.get(cur)) touched.add(cur);
    }
    for (const k of touched) {
      const node = nodes.get(k);
      if (node) bump(node, finding);
    }
  }

  const out = [...nodes.values()].filter((node) => node.total > 0);
  if (unassigned.total > 0) out.push(unassigned);
  out.sort((left, right) => {
    if ((left.kind === "none") !== (right.kind === "none")) return left.kind === "none" ? 1 : -1;
    return right.total - left.total || left.label.localeCompare(right.label);
  });
  return out;
}

// mockAssetScopeIds Place the asset tree node key Expand assets into the entire subtree id Collection,With the backend
// applyAssetScope Synonyms.miss=true Indicates that the node does not exist under the current filter → The result is empty.
function mockAssetScopeIds(
  scope: string,
  list: (typeof mockFindings)[number][],
): { ids: Set<string>; none: boolean; miss: boolean } {
  if (scope === UNASSIGNED_ASSET) return { ids: new Set(), none: true, miss: false };
  const nodes = mockBuildAssetTree(list);
  if (!nodes.some((node) => node.key === scope)) return { ids: new Set(), none: false, miss: true };
  const children = new Map<string, MockAssetTreeNode[]>();
  for (const node of nodes) {
    if (!node.parent) continue;
    children.set(node.parent, [...(children.get(node.parent) ?? []), node]);
  }
  const ids = new Set<string>();
  const queue = [scope];
  const seen = new Set(queue);
  while (queue.length > 0) {
    const cur = queue.shift() as string;
    const node = nodes.find((candidate) => candidate.key === cur);
    if (node?.asset_id) ids.add(String(node.asset_id));
    for (const child of children.get(cur) ?? []) {
      if (seen.has(child.key)) continue;
      seen.add(child.key);
      queue.push(child.key);
    }
  }
  return { ids, none: false, miss: ids.size === 0 };
}

// mockApplyAssetScope Press asset_scope Narrow a batch of findings.[Not associated]Collect at the same time assets Empty
// Discovery with links to deleted assets,Same caliber as the barrel on the tree.
function mockApplyAssetScope(
  list: (typeof mockFindings)[number][],
  scope: string | null,
): (typeof mockFindings)[number][] {
  if (!scope) return list;
  const { ids, none, miss } = mockAssetScopeIds(scope, list);
  if (none) {
    return list.filter((finding) => {
      const refs = finding.assets ?? [];
      return refs.length === 0 || refs.every((ref) => !mockAssets.some((a) => String(a.id) === ref.id));
    });
  }
  if (miss) return [];
  return list.filter((finding) => (finding.assets ?? []).some((ref) => ids.has(ref.id)));
}

function mockTaskCategorySnapshot(): TaskCategory[] {
  return mockTaskCategories.map((category) => ({
    ...category,
    task_count: mockTasks.filter((task) => task.category_id === category.id).length,
  }));
}

function mockScopeRows(
  companyID: number,
  input: unknown,
  existing: ScopeRow[] = [],
): { rows: ScopeRow[]; invalid: number; skipped: number } {
  if (!Array.isArray(input)) return { rows: [], invalid: 0, skipped: 0 };
  let nextID =
    mockCompanies.flatMap((company) => company.scope ?? []).reduce((max, row) => Math.max(max, row.id), 0) + 1;
  const rows: ScopeRow[] = [];
  let invalid = 0;
  let skipped = 0;
  const keys = new Set(
    existing.map((row) => `${row.kind}|${row.domain ?? row.net ?? row.value ?? row.raw.trim().toLowerCase()}`),
  );
  for (const [index, candidate] of input.entries()) {
    let rule: CompanyScopeRule | undefined;
    if (typeof candidate === "string") {
      rule = classifyCompanyScopeLine(candidate, index + 1).rule;
    } else if (candidate && typeof candidate === "object") {
      const item = candidate as { kind?: unknown; value?: unknown };
      const value = String(item.value ?? "").trim();
      if (item.kind === undefined || item.kind === "") rule = classifyCompanyScopeLine(value, index + 1).rule;
      else if (isCompanyScopeKind(item.kind)) rule = { kind: item.kind, value };
    }
    if (!rule || companyScopeRuleError(rule)) {
      invalid++;
      continue;
    }
    const normalized = normalizeCompanyScopeValue(rule);
    const row: ScopeRow = { id: nextID++, company_id: companyID, kind: rule.kind, raw: rule.value.trim() };
    if (rule.kind === "domain") row.domain = normalized;
    else if (rule.kind === "ip") row.net = `${normalized}/${normalized.includes(":") ? 128 : 32}`;
    else if (rule.kind === "cidr") row.net = normalized;
    else row.value = normalized;
    const key = `${row.kind}|${row.domain ?? row.net ?? row.value}`;
    if (keys.has(key)) {
      skipped++;
      continue;
    }
    keys.add(key);
    rows.push(row);
  }
  return { rows, invalid, skipped };
}

function mockProfileResolution(profileID: number | undefined, source: TaskLLMResolution["source"]): TaskLLMResolution {
  const profile = D.llmProfiles.find((item) => Number(item.id) === profileID);
  if (!profile) {
    return { name: "", format: "", model: "", source, available: false, reason: "LLM configuration does not exist" };
  }
  return {
    profile_id: Number(profile.id),
    name: profile.name,
    format: profile.format,
    model: profile.model,
    source,
    available: Boolean(profile.api_key_hint),
    reason: profile.api_key_hint ? undefined : "LLM configuration does not set API Key",
  };
}

// Mirrors the backend precedence in server/task_resolution.go:
// Agent Binding → Task LLM Configuration chain → Global configuration → Environment configuration.
function mockRoleResolution(task: Task, agentKey: "mainagent" | "planner" | "worker"): TaskLLMResolution {
  const agent = D.agents.find((item) => item.key === agentKey);
  if (agent?.llm_profile_id) {
    const bound = mockProfileResolution(agent.llm_profile_id, "agent_binding");
    if (bound.available) return bound;
  }
  if (task.llm_profile_ids?.length) {
    if (task.active_llm_profile_id === undefined) {
      return {
        name: "",
        format: "",
        model: "",
        source: "task_chain",
        available: false,
        reason: "Task LLM configuration chain quota has been exhausted",
      };
    }
    return mockProfileResolution(task.active_llm_profile_id, "task_chain");
  }
  const globalProfile = D.llmProfiles.find((item) => item.is_default);
  if (globalProfile) return mockProfileResolution(Number(globalProfile.id), "global_profile");
  return {
    name: "Global configuration",
    format: D.llmConfig.provider,
    model: D.llmConfig.model,
    source: "environment",
    available: true,
  };
}

function bodyIDs(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  const ids = [...new Set(value.map((item) => String(item)).filter(Boolean))];
  if (ids.length > 100) throw new Error("Batch operation supports up to 100 IDs");
  return ids;
}

type MockIntentControlResult = { ok: boolean; state?: string; error?: string };
type MockWorkerMessage = {
  intentId: string;
  message: string;
  state: "open" | "running";
  activitySeq: number;
};

const mockWorkerMessages = new Map<string, MockWorkerMessage>();
let nextMockWorkerMessageActivitySeq = mockActivity.reduce((maximum, item) => Math.max(maximum, item.seq), 0) + 1;

function controlMockIntent(id: string, action: "pause" | "resume"): MockIntentControlResult {
  const intent = mockIntents.find((item) => item.id === id);
  if (!intent) return { ok: false, error: "Intention does not exist" };
  const requiredState = action === "pause" ? "running" : "paused";
  if (intent.inherited || intent.state !== requiredState) {
    return { ok: false, state: intent.state, error: "Worker status has changed" };
  }
  intent.state = action === "pause" ? "paused" : "open";
  return { ok: true, state: intent.state };
}

function sendMockWorkerMessage(
  id: string,
  message: string,
  requestId: string,
): MockIntentControlResult & { activitySeq?: number; requestId?: string } {
  const normalizedMessage = message.trim();
  const normalizedRequestId = requestId.trim();
  if (!normalizedRequestId) return { ok: false, error: "request_id cannot be empty" };
  const previous = mockWorkerMessages.get(normalizedRequestId);
  if (previous) {
    if (previous.intentId !== id || previous.message !== normalizedMessage) {
      return { ok: false, error: "request_id has been used in other Worker messages" };
    }
    return {
      ok: true,
      state: previous.state,
      activitySeq: previous.activitySeq,
      requestId: normalizedRequestId,
    };
  }
  const intent = mockIntents.find((item) => item.id === id);
  if (!intent) return { ok: false, error: "Intention does not exist" };
  if (intent.inherited || intent.state !== "paused") {
    return { ok: false, state: intent.state, error: "Only suspended Workers can send messages, please pause first" };
  }
  if (!normalizedMessage) return { ok: false, state: intent.state, error: "Message cannot be empty" };
  if (Array.from(normalizedMessage).length > 4000) {
    return { ok: false, state: intent.state, error: "Message cannot exceed 4000 characters" };
  }

  // The real endpoint transitions the intent paused->running, records the user turn,
  // and runs it in a dedicated goroutine outside the worker pool. Keep the mock in
  // sync: flip to running immediately and emit the visible user activity.
  intent.state = "running";
  const activitySeq = nextMockWorkerMessageActivitySeq++;
  const workerMessage: MockWorkerMessage = {
    intentId: id,
    message: normalizedMessage,
    state: "running",
    activitySeq,
  };
  mockWorkerMessages.set(normalizedRequestId, workerMessage);
  mockActivity.push({
    seq: activitySeq,
    intent_id: id,
    worker: "user",
    ts: new Date().toISOString(),
    kind: "user",
    summary: normalizedMessage,
    detail: normalizedMessage,
  } satisfies Activity);
  return {
    ok: true,
    state: workerMessage.state,
    activitySeq: workerMessage.activitySeq,
    requestId: normalizedRequestId,
  };
}

function controlMockTask(id: string, action: "pause" | "resume"): BatchControlItem {
  const task = mockTasks.find((item) => item.id === id);
  if (!task) return { id, ok: false, error: "Task does not exist" };
  if (task.status === "done" || task.status === "failed" || task.status === "timeout") {
    return { id, ok: false, status: task.status, error: "The final task is uncontrollable" };
  }
  if (action === "pause") {
    if (task.paused || task.status === "paused") {
      return { id, ok: false, status: task.status, error: "The task has been suspended" };
    }
    task.paused = true;
    task.queued = false;
    task.status = "paused";
    task.engine_mode = "paused";
  } else {
    if (!task.paused && task.status !== "paused") {
      return { id, ok: false, status: task.status, error: "The task is not paused" };
    }
    task.paused = false;
    task.queued = false;
    task.status = "running";
    task.engine_mode = "exploring";
  }
  return { id, ok: true, status: task.status, queued: false };
}

function sortMockConversations() {
  mockConversations.sort((a, b) => {
    const aPinned = a.pinned_at ? 1 : 0;
    const bPinned = b.pinned_at ? 1 : 0;
    if (aPinned !== bPinned) return bPinned - aPinned;
    const aTime = a.pinned_at ?? a.updated_at;
    const bTime = b.pinned_at ?? b.updated_at;
    return bTime.localeCompare(aTime) || b.id - a.id;
  });
}

function sortMockTasks() {
  mockTasks.sort((a, b) => {
    const aPinned = a.pinned_at ? 1 : 0;
    const bPinned = b.pinned_at ? 1 : 0;
    if (aPinned !== bPinned) return bPinned - aPinned;
    if (a.pinned_at !== b.pinned_at) return (b.pinned_at ?? "").localeCompare(a.pinned_at ?? "");
    return b.id.localeCompare(a.id, undefined, { numeric: true });
  });
}

function normalizedTemplateName(value: unknown): string {
  return String(value ?? "")
    .trim()
    .split(/\s+/)
    .join(" ");
}

function parseBody(body?: BodyInit | null): Record<string, unknown> {
  if (typeof body !== "string") return {};
  try {
    return JSON.parse(body) as Record<string, unknown>;
  } catch {
    return {};
  }
}

function mockRequestedDays(raw: string | null, fallback: number): number {
  const parsed = Number(raw);
  const days = Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback;
  return Math.min(366, Math.max(1, days));
}

// Daily buckets end today so dashboard ranges of 7, 30, 90, 180, and 365 all have points.
function mockUsageDaily(days: number) {
  const profile = D.llmProfiles.find((item) => item.is_default)?.name ?? "default";
  return Array.from({ length: days }, (_, index) => {
    const day = new Date();
    day.setUTCHours(0, 0, 0, 0);
    day.setUTCDate(day.getUTCDate() - (days - 1 - index));
    const wave = 20000 + Math.round(15000 * Math.abs(Math.sin(index / 3)));
    return {
      profile_name: profile,
      date: day.toISOString().slice(0, 10),
      input_tokens: wave,
      output_tokens: Math.round(wave * 0.08),
      cache_read_tokens: Math.round(wave * 0.7),
    };
  });
}

// The fixture covers 30 days. Longer ranges keep those days and extend backward.
function mockDailyTokens(days: number) {
  if (days <= D.dailyTokens.length) return D.dailyTokens.slice(-days);
  const extra = days - D.dailyTokens.length;
  const first = new Date(`${D.dailyTokens[0].date}T00:00:00Z`);
  const older = Array.from({ length: extra }, (_, index) => {
    const day = new Date(first);
    day.setUTCDate(day.getUTCDate() - (extra - index));
    const wave = 40000 + Math.round(30000 * Math.abs(Math.sin(index / 3)));
    return {
      date: day.toISOString().slice(0, 10),
      input_tokens: wave,
      output_tokens: Math.round(wave * 0.08),
      cache_read_tokens: Math.round(wave * 0.7),
      cache_write_tokens: Math.round(wave * 0.12),
    };
  });
  return [...older, ...D.dailyTokens];
}

function mockGoalCards() {
  return D.explorationGraph.nodes
    .filter((node) => node.type === "goal")
    .map((node) => {
      let text = node.id;
      try {
        if (node.payload) {
          const payload = JSON.parse(node.payload) as { text?: string };
          if (payload.text) text = payload.text;
        }
      } catch {
        // Keep the node id when the payload is not JSON.
      }
      return {
        id: node.id,
        text,
        vulnclass: "",
        state: node.state,
        origin: node.origin,
        ts: node.ts,
      };
    });
}

function mockActivitySessionKey(item: Activity): string {
  if (item.worker === "system" || item.kind === "llm_switch" || item.kind === "llm_failover") return "system";
  if (item.worker === "mainagent") return `main:${item.main_seg ?? 0}`;
  if (item.worker === "planner") return "plan";
  if (item.intent_id) return `intent:${item.intent_id}`;
  return "unknown";
}

function mockActivityHistory(q: URLSearchParams) {
  const session = q.get("session") || "main:0";
  const before = Number(q.get("before") ?? 0);
  const limit = Math.min(500, Math.max(1, Number(q.get("limit") ?? 200)));
  const matched = mockActivity.filter((item) => {
    const key = mockActivitySessionKey(item);
    return key === session || (session === "main" && key === "main:0");
  });
  const ordered = matched.filter((item) => before <= 0 || item.seq < before).sort((a, b) => a.seq - b.seq);
  const page = ordered.slice(Math.max(0, ordered.length - limit));
  const snapshot = mockActivity.reduce((max, item) => Math.max(max, item.seq), 0);
  return {
    items: page,
    snapshot_cursor: snapshot,
    earliest_cursor: page.length ? page[0].seq : before,
    has_more: ordered.length > page.length,
  };
}

function mockMainSessions() {
  const bySeg = new Map<number, string>();
  for (const item of mockActivity) {
    if (item.worker !== "mainagent") continue;
    const seg = item.main_seg ?? 0;
    const previous = bySeg.get(seg);
    if (!previous || item.ts < previous) bySeg.set(seg, item.ts);
  }
  if (bySeg.size === 0) bySeg.set(0, "2026-07-24T09:12:00Z");
  const sessions = [...bySeg.entries()]
    .sort((left, right) => right[0] - left[0])
    .map(([seq, created_at]) => ({ seq, created_at }));
  return { sessions, current: sessions[0]?.seq ?? 0 };
}

const mockRetryRule = { attempts: 0, interval_ms: 0 };

const mockNotifyMeta = {
  kinds: [
    { kind: "dingtalk", default_rate_per_min: 20, secret_keys: ["webhook", "secret"] },
    { kind: "email", default_rate_per_min: 60, secret_keys: ["password"] },
    { kind: "feishu", default_rate_per_min: 100, secret_keys: ["webhook", "secret"] },
    { kind: "telegram", default_rate_per_min: 20, secret_keys: ["bot_token"] },
    { kind: "webhook", default_rate_per_min: 0, secret_keys: ["url", "headers"] },
    { kind: "wecom", default_rate_per_min: 20, secret_keys: ["webhook"] },
  ],
  enabled: true,
  public_base_url: "",
  digest_interval_min: "30",
  defaults: { digest_interval_min: 30 },
  stats: {
    channels: 0,
    channels_on: 0,
    pending: 0,
    failed: 0,
    sent_today: 0,
    backlog_age_ms: 0,
  },
};

// Path templates verified against the UI client. Tests walk this array; do not keep a second copy.
export const MOCK_GET_ROUTES = [
  "/health",
  "/auth/status",
  "/tasks",
  "/tasks/:taskId",
  "/task-categories",
  "/task-templates",
  "/task-archives",
  "/task-archives/:archiveId",
  "/tasks/:taskId/llm/resolution",
  "/stats",
  "/tasks/:taskId/coverage",
  "/tasks/:taskId/coverage-graph",
  "/tokens/usage?days=365",
  "/tokens/daily?days=90",
  "/tokens/conversations",
  "/tasks/:taskId/goals",
  "/tasks/:taskId/constraints",
  "/tasks/:taskId/intercept-rules",
  "/tasks/:taskId/scope",
  "/tasks/:taskId/asset-refs?asset_id=1",
  "/workspace/list",
  "/workspace/read",
  "/assets",
  "/assets/counts",
  "/tasks/:taskId/intent-assets",
  "/companies",
  "/exploration/frontier",
  "/exploration/findings",
  "/exploration/findings/groups",
  "/exploration/findings/asset-tree",
  "/exploration/findings/stats",
  "/exploration/findings/:findingId",
  "/exploration/findings/:findingId/traffic",
  "/exploration/findings/:findingId/traffic/:bindingId",
  "/exploration/findings/:findingId/traffic/:bindingId/body",
  "/exploration/findings/:findingId/lineage",
  "/exploration/findings/:findingId/retests",
  "/exploration/findings/retests/active",
  "/exploration/intents",
  "/exploration/tokens",
  "/exploration/graph",
  "/exploration/nodes",
  "/exploration/activity",
  "/exploration/activity/history?session=main:0",
  "/exploration/activity/:activitySeq",
  "/exploration/main-sessions",
  "/audit",
  "/traffic",
  "/traffic/exchange?id=x-2",
  "/traffic/hosts",
  "/settings",
  "/notify/meta",
  "/notify/channels",
  "/notify/deliveries",
  "/chat/mentions",
  "/tasks/:taskId/chat/status",
  "/tasks/:taskId/chat/side-questions",
  "/tasks/:taskId/intents/:intentId/side-questions",
  "/llm",
  "/llm/profiles",
  "/llm/pool",
  "/llm/retry-policy",
  "/llm/records",
  "/llm/records/tasks",
  "/llm/records/by-model?task=:taskId",
  "/llm/records/:recordId",
  "/agents",
  "/agents/:agentKey",
  "/agents/:agentKey/triggers",
  "/agents/:agentKey/prompts",
  "/agents/:agentKey/variables",
  "/agents/:agentKey/visibility",
  "/conversations",
  "/conversations/:conversationId/messages",
  "/conversations/:conversationId/messages/:messageSeq",
  "/conversations/:conversationId/side-questions",
  "/tools",
  "/mcp",
  "/mcp/:mcpId/tools",
  "/sync/scopesentry/status",
  "/sync/scopesentry/projects",
  "/sync/scopesentry/tasks",
  "/skills",
  "/skills/missing",
  "/skills/:skillName/files",
  "/skills/:skillName/files/:skillFile",
  "/skills/:skillName/usage",
  "/visibility/mcp/:mcpId",
  "/visibility/skill/:skillName",
  "/intercept/rules",
  "/asset-intercept/rules",
  "/intercept/pending",
  "/intercept/pending/:pendingId",
  "/intercept/history",
  "/intercept/history/:approvalId",
  "/intercept/history/:approvalId/execution",
  "/intercept/task/:taskId",
  "/intercept/tool-config",
  "/intercept/judge",
  "/intercept/judge/usage",
  "/commands",
  "/commands/stats",
  "/update/check",
  "/logs/history",
];

function mockTrafficList(q: URLSearchParams) {
  const all = D.traffic.exchanges ?? [];
  const host = q.get("host") ?? "";
  const method = q.get("method") ?? "";
  const query = (q.get("q") ?? "").toLowerCase();
  const pathQuery = q.get("path") ?? "";
  const status = q.get("status") ?? "";
  const bodyQuery = (q.get("body") ?? "").toLowerCase();
  const respMin = q.get("resp_min");
  const respMax = q.get("resp_max");
  const rows = all.filter((row) => {
    if (host && row.host !== host) return false;
    if (method && method !== "all" && row.method !== method) return false;
    if (status && status !== "all" && String(row.status) !== status) return false;
    if (query && !`${row.host} ${row.url}`.toLowerCase().includes(query)) return false;
    if (pathQuery && !row.url.includes(pathQuery)) return false;
    if (respMin && row.resp_len < Number(respMin)) return false;
    if (respMax && row.resp_len > Number(respMax)) return false;
    if (bodyQuery) {
      const body = D.trafficBodies[row.id];
      const blob = `${body?.req ?? ""}\n${body?.resp ?? ""}`.toLowerCase();
      if (!blob.includes(bodyQuery)) return false;
    }
    return true;
  });
  const page = Math.max(0, Number(q.get("page") ?? 0) || 0);
  const size = Math.max(1, Number(q.get("size") ?? 100) || 100);
  return {
    enabled: D.traffic.enabled,
    proxy: D.traffic.proxy,
    count: all.length,
    total: rows.length,
    page,
    size,
    exchanges: rows.slice(page * size, page * size + size),
  };
}

function mockFilterCommands(q: URLSearchParams) {
  const task = q.get("task") ?? "";
  const query = (q.get("q") ?? "").toLowerCase();
  return D.commandRecords.filter((row) => {
    if (task) {
      const numeric = Number(task);
      const numericMatch = Number.isInteger(numeric) && (row.exploration_id === numeric || D.tasks[numeric - 1]?.id === row.task_id);
      if (row.task_id !== task && !numericMatch) return false;
    }
    if (query && !`${row.tool} ${row.command} ${row.output}`.toLowerCase().includes(query)) return false;
    return true;
  });
}

function mockCommandPage(q: URLSearchParams) {
  const rows = mockFilterCommands(q);
  const page = Math.max(0, Number(q.get("page") ?? 0) || 0);
  const size = Math.max(1, Number(q.get("size") ?? 50) || 50);
  return { commands: rows.slice(page * size, page * size + size), total: rows.length };
}

function mockLLMRecordPage(q: URLSearchParams) {
  const task = q.get("task") ?? "";
  const model = (q.get("model") ?? "").toLowerCase();
  const session = (q.get("session") ?? "").toLowerCase();
  const rows = mockLLMRecords.filter((row) => {
    if (task && row.task_id !== task) return false;
    if (model && !row.model.toLowerCase().includes(model)) return false;
    if (session && !row.session_id.toLowerCase().includes(session)) return false;
    return true;
  });
  const page = Math.max(0, Number(q.get("page") ?? 0) || 0);
  const size = Math.max(1, Number(q.get("size") ?? 50) || 50);
  return { records: rows.slice(page * size, page * size + size), total: rows.length };
}

function mockJudgeUsage(days: number) {
  const daily = Array.from({ length: days }, (_, index) => {
    const when = new Date();
    when.setUTCHours(0, 0, 0, 0);
    when.setUTCDate(when.getUTCDate() - (days - 1 - index));
    const calls = 1 + (index % 3);
    return {
      date: when.toISOString().slice(0, 10),
      calls,
      input_tokens: calls * 20,
      output_tokens: calls * 8,
    };
  });
  return {
    calls: daily.reduce((sum, row) => sum + row.calls, 0),
    input_tokens: daily.reduce((sum, row) => sum + row.input_tokens, 0),
    output_tokens: daily.reduce((sum, row) => sum + row.output_tokens, 0),
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    daily,
  };
}

function mockNotifyDeliveries(q: URLSearchParams) {
  const channelID = Number(q.get("channel_id") ?? 0);
  const state = q.get("state") ?? "";
  const rows = D.notifyDeliveries.filter((row) => {
    if (channelID && row.channel_id !== channelID) return false;
    if (state && row.state !== state) return false;
    return true;
  });
  const page = Math.max(1, Number(q.get("page") ?? 1) || 1);
  const pageSize = Math.min(200, Math.max(1, Number(q.get("page_size") ?? 50) || 50));
  return {
    deliveries: rows.slice((page - 1) * pageSize, page * pageSize),
    total: rows.length,
    page,
    page_size: pageSize,
  };
}

function mockCampaignLogs(q: URLSearchParams) {
  const before = Number(q.get("before") ?? 0);
  const limit = Math.min(500, Math.max(1, Number(q.get("limit") ?? 200) || 200));
  const rows = D.campaignLogs.filter((line) => !before || (line.db_id ?? 0) < before);
  const items = rows.slice(-limit);
  return { items, has_more: rows.length > items.length };
}

export async function mockHandle<T>(method: string, rawPath: string, body?: BodyInit | null): Promise<T> {
  advanceMockRetests();
  await delay();
  const [path, qs] = rawPath.split("?");
  const q = new URLSearchParams(qs ?? "");
  const seg = path.split("/").filter(Boolean); // ["exploration","activity"]
  const m = method.toUpperCase();
  const b = parseBody(body);
  return route(m, path, seg, q, b) as T;
}

function route(m: string, path: string, seg: string[], q: URLSearchParams, b: Record<string, unknown>): unknown {
  const task = q.get("task") ?? undefined;

  if (path === "/chat/mentions" && m === "GET") {
    const kind = q.get("kind") ?? "";
    const query = (q.get("q") ?? "").trim().toLowerCase();
    const candidates = [
      ...D.findings.map((finding, index) => ({
        kind: "finding",
        id: index + 1,
        label: finding.name || finding.vulnclass,
        description: `${finding.severity} · ${finding.summary}`,
      })),
      ...D.companies.map((company) => ({ kind: "company", id: company.id, label: company.name, description: "Enterprise" })),
      ...D.assets.map((asset) => ({
        kind: asset.type,
        id: asset.id,
        label:
          asset.type === "endpoint"
            ? `${asset.method || "GET"} ${asset.url}`
            : asset.app_name || asset.url || asset.domain || asset.ip || asset.bundle_id || `Asset #${asset.id}`,
        description: [asset.type, asset.page_title, asset.service_name, asset.bundle_id, asset.ip]
          .filter(Boolean)
          .join(" · "),
      })),
    ];
    const filtered = candidates
      .filter(
        (item) =>
          (!kind || kind === item.kind || (kind === "asset" && item.kind !== "finding" && item.kind !== "company")) &&
          (!query || String(item.id) === query || `${item.label} ${item.description}`.toLowerCase().includes(query)),
      )
      .sort(
        (a, b) =>
          Number(String(b.id) === query) - Number(String(a.id) === query) ||
          b.id - a.id ||
          a.kind.localeCompare(b.kind),
      );
    const offset = Math.max(0, Number(q.get("cursor")) || 0);
    return {
      items: filtered.slice(offset, offset + 20),
      next_cursor: offset + 20 < filtered.length ? String(offset + 20) : undefined,
    };
  }

  // ── auth:Let demo Go directly to the main interface ──
  if (path === "/auth/status") return { initialized: true };
  if (path === "/health") return { ok: true, service: "artex", version: "dev" };
  if (path === "/auth/login" || path === "/auth/init") return { token: "mock-demo" };
  if (path === "/auth/change-password") return { ok: true };

  // ── task cold archives ──
  if (path === "/task-archives" && m === "GET") {
    const page = Math.max(1, Number(q.get("page")) || 1);
    const size = Math.min(100, Math.max(1, Number(q.get("size")) || 20));
    const query = (q.get("q") ?? "").trim().toLowerCase();
    const state = (q.get("state") ?? "").trim();
    const filtered = mockTaskArchives.filter((archive) => {
      if (state && archive.state !== state) return false;
      if (!query) return true;
      return [archive.task_id, archive.task_name, archive.task_description].some((value) =>
        String(value ?? "")
          .toLowerCase()
          .includes(query),
      );
    });
    const offset = (page - 1) * size;
    return {
      items: filtered.slice(offset, offset + size).map(publicMockTaskArchive),
      total: filtered.length,
      page,
      size,
    };
  }
  if (seg[0] === "task-archives" && seg.length === 2 && m === "GET") {
    const archive = mockTaskArchives.find((item) => item.id === Number(seg[1]));
    if (!archive) throw new Error("Archive does not exist");
    return publicMockTaskArchive(archive);
  }
  if (seg[0] === "task-archives" && seg[2] === "restore" && seg.length === 3 && m === "POST") {
    const archive = mockTaskArchives.find((item) => item.id === Number(seg[1]));
    if (!archive) throw new Error("Archive does not exist");
    mockRestoreArchive(archive);
    return publicMockTaskArchive(archive);
  }
  if (seg[0] === "task-archives" && seg.length === 2 && m === "DELETE") {
    const archive = mockTaskArchives.find((item) => item.id === Number(seg[1]));
    if (!archive) throw new Error("Archive does not exist");
    mockDeleteArchive(archive);
    return publicMockTaskArchive(archive);
  }
  if (path === "/task-archives/restore/batch" && m === "POST") {
    const ids = bodyIDs(b.archive_ids).map(Number);
    const items = ids.map<ArchiveBatchItem>((id) => {
      const archive = mockTaskArchives.find((item) => item.id === id);
      if (!archive) return { id: String(id), archive_id: id, ok: false, queued: false, error: "Archive does not exist" };
      try {
        mockRestoreArchive(archive);
        return { id: String(id), archive_id: id, ok: true, queued: true };
      } catch (error) {
        return { id: String(id), archive_id: id, ok: false, queued: false, error: (error as Error).message };
      }
    });
    return { items };
  }
  if (path === "/task-archives/delete/batch" && m === "POST") {
    const ids = bodyIDs(b.archive_ids).map(Number);
    const items = ids.map<ArchiveBatchItem>((id) => {
      const archive = mockTaskArchives.find((item) => item.id === id);
      if (!archive) return { id: String(id), archive_id: id, ok: false, queued: false, error: "Archive does not exist" };
      try {
        mockDeleteArchive(archive);
        return { id: String(id), archive_id: id, ok: true, queued: true };
      } catch (error) {
        return { id: String(id), archive_id: id, ok: false, queued: false, error: (error as Error).message };
      }
    });
    return { items };
  }
  if (path === "/tasks/archive/batch" && m === "POST") {
    const requested = bodyIDs(b.task_ids);
    const selected = new Set(requested);
    const ordered: string[] = [];
    const visiting = new Set<string>();
    const visited = new Set<string>();
    const visit = (id: string) => {
      if (visited.has(id) || visiting.has(id)) return;
      visiting.add(id);
      for (const candidate of mockTasks) {
        if (!selected.has(candidate.id) || !(candidate.source_task_ids ?? []).map(String).includes(id)) continue;
        visit(candidate.id);
      }
      visiting.delete(id);
      visited.add(id);
      ordered.push(id);
    };
    for (const id of requested) visit(id);
    const byID = new Map<string, ArchiveBatchItem>();
    for (const id of ordered) {
      try {
        const archive = mockArchiveTask(id);
        byID.set(id, { id, archive_id: archive.id, ok: true, queued: true });
      } catch (error) {
        byID.set(id, { id, ok: false, queued: false, error: (error as Error).message });
      }
    }
    return { items: requested.map((id) => byID.get(id) ?? { id, ok: false, queued: false, error: "Task does not exist" }) };
  }
  if (seg[0] === "tasks" && seg[2] === "archive" && seg.length === 3 && m === "POST") {
    return publicMockTaskArchive(mockArchiveTask(seg[1]));
  }

  // ── tasks ──
  if (path === "/tasks" && m === "GET") {
    sortMockTasks();
    return { tasks: mockTasks.map(publicMockTask), active: mockActiveTask };
  }
  if (path === "/tasks" && m === "POST") {
    let suffix = 1;
    while (mockTasks.some((item) => item.id === `t-new-${suffix}`)) suffix++;
    const id = `t-new-${suffix}`;
    const now = new Date();
    const profileIDs = [...((b.llm_profile_ids as number[] | undefined) ?? [])];
    const sourceTaskIDs = [...((b.source_task_ids as string[] | undefined) ?? [])];
    const companyIDs = [...new Set((b.company_ids as number[] | undefined) ?? [])];
    if (companyIDs.some((companyID) => !mockCompanies.some((company) => company.id === companyID))) {
      throw new Error("The associated enterprise does not exist or is invalid");
    }
    const categoryID = typeof b.category_id === "number" ? b.category_id : undefined;
    const category = categoryID === undefined ? undefined : mockTaskCategories.find((item) => item.id === categoryID);
    if (categoryID !== undefined && !category) throw new Error("Task classification does not exist");
    const created: Task = {
      id,
      name: String(b.name ?? ""),
      category_id: category?.id,
      category_name: category?.name,
      description: String(b.description ?? "New tasks"),
      goal: String(b.goal ?? ""),
      status: "created",
      created_at: now.toISOString(),
      created_unix: Math.floor(now.getTime() / 1000),
      paused: false,
      active: true,
      in_flight: 0,
      findings: { critical: 0, high: 0, medium: 0, low: 0 },
      stalled: false,
      goals_total: 0,
      goals_met: 0,
      engine_mode: "idle",
      tokens: { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0 },
      llm_profile_id: profileIDs[0],
      llm_profile_ids: profileIDs,
      active_llm_profile_id: profileIDs[0],
      llm_failover_state: profileIDs.length ? "ready" : "default",
      source_task_ids: sourceTaskIDs,
      company_ids: companyIDs,
    };
    const numericTaskID = nextMockTaskAssetID++;
    mockTaskAssetIDs.set(id, numericTaskID);
    const selectedCompanies = new Set(companyIDs);
    for (const asset of mockAssets) {
      if (asset.company_id === undefined || !selectedCompanies.has(asset.company_id)) continue;
      if (!asset.task_ids.includes(numericTaskID)) asset.task_ids.push(numericTaskID);
      const company = mockCompanies.find((candidate) => candidate.id === asset.company_id);
      setMockTaskAssetSource(id, asset.id, {
        task_source: "company",
        task_source_summary: `Company linked when the task was created: ${company?.name ?? `#${asset.company_id}`}`,
        task_source_node_id: undefined,
      });
    }
    for (const item of mockTasks) item.active = false;
    mockTasks.unshift(created);
    mockActiveTask = id;
    return created;
  }
  if (path === "/task-categories" && m === "GET") return { categories: mockTaskCategorySnapshot() };
  if (path === "/task-categories" && m === "POST") {
    const name = normalizedTemplateName(b.name);
    if (!name) throw new Error("Category name cannot be empty");
    if (mockTaskCategories.some((category) => category.name.toLowerCase() === name.toLowerCase())) {
      throw new Error("Category name already exists");
    }
    const now = new Date().toISOString();
    const category: TaskCategory = {
      id: mockTaskCategories.reduce((maximum, item) => Math.max(maximum, item.id), 0) + 1,
      name,
      task_count: 0,
      created_at: now,
      updated_at: now,
    };
    mockTaskCategories.push(category);
    return category;
  }
  if (seg[0] === "task-categories" && seg.length === 2 && m === "PATCH") {
    const category = mockTaskCategories.find((item) => item.id === Number(seg[1]));
    if (!category) throw new Error("Task classification does not exist");
    const name = normalizedTemplateName(b.name);
    if (!name) throw new Error("Category name cannot be empty");
    if (mockTaskCategories.some((item) => item.id !== category.id && item.name.toLowerCase() === name.toLowerCase())) {
      throw new Error("Category name already exists");
    }
    category.name = name;
    category.updated_at = new Date().toISOString();
    for (const task of mockTasks) {
      if (task.category_id === category.id) task.category_name = name;
    }
    return { ...category, task_count: mockTasks.filter((task) => task.category_id === category.id).length };
  }
  if (seg[0] === "task-categories" && seg.length === 2 && m === "DELETE") {
    const categoryID = Number(seg[1]);
    const index = mockTaskCategories.findIndex((item) => item.id === categoryID);
    if (index < 0) throw new Error("Task classification does not exist");
    mockTaskCategories.splice(index, 1);
    for (const task of mockTasks) {
      if (task.category_id !== categoryID) continue;
      task.category_id = undefined;
      task.category_name = undefined;
    }
    return { deleted: categoryID };
  }
  if (path === "/tasks/category/batch" && m === "POST") {
    const requested = Array.isArray(b.task_ids) ? b.task_ids.map(String) : [];
    const taskIDs = [...new Set(requested)];
    if (taskIDs.length === 0 || taskIDs.length > 100) throw new Error("task_ids quantity must be 1-100");
    const categoryID = typeof b.category_id === "number" ? b.category_id : undefined;
    const category = categoryID === undefined ? undefined : mockTaskCategories.find((item) => item.id === categoryID);
    if (categoryID !== undefined && !category) throw new Error("Task classification does not exist");
    const items = taskIDs.map((id) => {
      const task = mockTasks.find((item) => item.id === id);
      if (!task) return { id, ok: false, error: "task not found" };
      task.category_id = category?.id;
      task.category_name = category?.name;
      return { id, ok: true };
    });
    return {
      items,
      category: category ? mockTaskCategorySnapshot().find((item) => item.id === category.id) : null,
    };
  }
  if (seg[0] === "tasks" && seg[2] === "category" && seg.length === 3 && m === "PATCH") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("Task does not exist");
    const categoryID = typeof b.category_id === "number" ? b.category_id : undefined;
    const category = categoryID === undefined ? undefined : mockTaskCategories.find((item) => item.id === categoryID);
    if (categoryID !== undefined && !category) throw new Error("Task classification does not exist");
    task.category_id = category?.id;
    task.category_name = category?.name;
    return task;
  }
  if (path === "/task-templates" && m === "GET") return { templates: mockTaskTemplates };
  if (path === "/task-templates" && m === "POST") {
    const now = new Date().toISOString();
    const name = normalizedTemplateName(b.name);
    const description = String(b.description ?? "").trim();
    const goal = String(b.goal ?? "").trim();
    if (!name || !description || !goal) throw new Error("Please fill in the template name, description and goal");
    if (
      mockTaskTemplates.some((template) => normalizedTemplateName(template.name).toLowerCase() === name.toLowerCase())
    ) {
      throw new Error("Template name already exists");
    }
    const nextID = mockTaskTemplates.reduce((max, template) => Math.max(max, template.id), 0) + 1;
    const created: TaskTemplate = {
      id: nextID,
      name,
      description,
      goal,
      created_at: now,
      updated_at: now,
    };
    mockTaskTemplates.unshift(created);
    return created;
  }
  if (seg[0] === "task-templates" && seg.length === 2 && m === "PATCH") {
    const template = mockTaskTemplates.find((item) => item.id === Number(seg[1]));
    if (!template) return {};
    const name = typeof b.name === "string" ? normalizedTemplateName(b.name) : template.name;
    const description = typeof b.description === "string" ? b.description.trim() : template.description;
    const goal = typeof b.goal === "string" ? b.goal.trim() : template.goal;
    if (!name || !description || !goal) throw new Error("Please fill in the template name, description and goal");
    if (
      mockTaskTemplates.some(
        (item) => item.id !== template.id && normalizedTemplateName(item.name).toLowerCase() === name.toLowerCase(),
      )
    ) {
      throw new Error("Template name already exists");
    }
    template.name = name;
    template.description = description;
    template.goal = goal;
    template.updated_at = new Date().toISOString();
    mockTaskTemplates.sort((a, b) => b.updated_at.localeCompare(a.updated_at) || b.id - a.id);
    return template;
  }
  if (seg[0] === "task-templates" && seg.length === 2 && m === "DELETE") {
    const id = Number(seg[1]);
    const index = mockTaskTemplates.findIndex((item) => item.id === id);
    if (index >= 0) mockTaskTemplates.splice(index, 1);
    return { deleted: id };
  }
  if (seg[0] === "tasks" && seg.length === 2 && m === "GET") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("Task does not exist");
    return publicMockTask(task);
  }
  if (seg[0] === "tasks" && seg.length === 2 && m === "PATCH") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("Task does not exist");
    if (typeof b.name === "string") task.name = b.name.trim();
    if (typeof b.pinned === "boolean") {
      task.pinned = b.pinned;
      task.pinned_at = b.pinned ? (task.pinned_at ?? new Date().toISOString()) : null;
    }
    sortMockTasks();
    return structuredClone(task);
  }
  if (seg[0] === "tasks" && seg.length === 2 && m === "DELETE") {
    const id = seg[1];
    const numericTaskID = mockTaskAssetID(id);
    const index = mockTasks.findIndex((item) => item.id === id);
    if (index >= 0) mockTasks.splice(index, 1);
    mockTaskAssetIDs.delete(id);
    deleteMockTaskAssetSources(id);
    if (numericTaskID !== undefined) {
      for (const asset of mockAssets) asset.task_ids = asset.task_ids.filter((taskID) => taskID !== numericTaskID);
    }

    let findingsDeleted = 0;
    if (b.delete_findings) {
      for (let i = mockFindings.length - 1; i >= 0; i--) {
        if (mockFindings[i].task_id !== id) continue;
        mockFindings.splice(i, 1);
        findingsDeleted++;
      }
    } else {
      // PostgreSQL uses ON DELETE SET NULL for retained findings. Keep the mock
      // grouped view consistent by moving them into the unassigned/deleted bucket.
      for (const finding of mockFindings) {
        if (finding.task_id !== id) continue;
        finding.task_id = undefined;
        finding.task_description = "";
      }
    }

    let llmRecordsDeleted = 0;
    if (b.delete_llm_records) {
      for (let i = mockLLMRecords.length - 1; i >= 0; i--) {
        if (mockLLMRecords[i].task_id !== id) continue;
        mockLLMRecords.splice(i, 1);
        llmRecordsDeleted++;
      }
    }

    if (mockActiveTask === id) {
      const nextActive = mockTasks[0]?.id ?? "";
      mockActiveTask = nextActive;
      for (const item of mockTasks) item.active = item.id === nextActive;
    }
    return {
      deleted: id,
      assets_deleted: b.delete_assets ? 1 : 0,
      assets_detached: 0,
      traffic_deleted: b.delete_traffic ? 1 : 0,
      files_deleted: Boolean(b.delete_files),
      findings_deleted: findingsDeleted,
      llm_records_deleted: llmRecordsDeleted,
    };
  }
  if (seg[0] === "tasks" && seg[2] === "llm" && m === "PUT") {
    const ids = [...((b.llm_profile_ids as number[] | undefined) ?? [])];
    const activeID = typeof b.active_llm_profile_id === "number" ? b.active_llm_profile_id : ids[0];
    const target = mockTasks.find((item) => item.id === seg[1]);
    if (target) {
      target.llm_profile_ids = ids;
      target.llm_profile_id = ids[0];
      target.active_llm_profile_id = activeID;
      target.llm_failover_state = ids.length ? "ready" : "default";
      target.llm_failover_reason = undefined;
    }
    return {
      id: seg[1],
      llm_profile_ids: ids,
      active_llm_profile_id: activeID,
      llm_failover_state: ids.length ? "ready" : "default",
      reopened_intents: 0,
    };
  }
  if (seg[0] === "tasks" && seg[2] === "llm" && seg[3] === "resolution" && m === "GET") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("Task does not exist");
    return {
      mainagent: mockRoleResolution(task, "mainagent"),
      planner: mockRoleResolution(task, "planner"),
      worker: mockRoleResolution(task, "worker"),
    };
  }
  if (path === "/tasks/control/batch" && m === "POST") {
    const action = b.action === "resume" ? "resume" : "pause";
    return { items: bodyIDs(b.task_ids).map((id) => controlMockTask(id, action)) };
  }
  if (seg[0] === "tasks" && seg[2] === "intents" && seg[4] === "control" && m === "POST") {
    const id = seg[3];
    if (b.action === "cancel") {
      const index = mockIntents.findIndex((item) => item.id === id);
      const intent = mockIntents[index];
      if (
        !intent ||
        intent.inherited ||
        (intent.state !== "running" && intent.state !== "paused" && intent.state !== "open")
      ) {
        throw new Error("Worker status has changed");
      }
      const numId = Number(id.replace(/\D/g, "")) || 0;
      if (b.mode === "hard") {
        // True delete:Remove from list,Return cascade delete count.
        mockIntents.splice(index, 1);
        return {
          id: numId,
          state: "",
          deleted: {
            intents: 1,
            facts: 1,
            findings: 1,
            activities: mockActivity.filter((item) => item.intent_id === id).length,
          },
        };
      }
      // Fake deletion(Default):set deleted + Remember the reason for deletion,Reserved node.
      intent.state = "deleted";
      intent.delete_reason = String(b.reason ?? "");
      return { id: numId, state: "deleted" };
    }
    const action = b.action === "resume" ? "resume" : "pause";
    const result = controlMockIntent(id, action);
    if (!result.ok) throw new Error(result.error ?? "Worker status has changed");
    return { id: Number(id.replace(/\D/g, "")) || 0, state: result.state };
  }
  if (seg[0] === "tasks" && seg[2] === "intents" && seg[4] === "messages" && m === "POST") {
    const id = seg[3];
    const result = sendMockWorkerMessage(id, String(b.message ?? ""), String(b.request_id ?? ""));
    if (!result.ok) throw new Error(result.error ?? "Worker status has changed");
    return {
      id: Number(id.replace(/\D/g, "")) || 0,
      state: result.state ?? "running",
      accepted: true,
      request_id: result.requestId ?? "",
    };
  }
  if (seg[0] === "tasks" && seg.length === 3 && seg[2] === "control" && m === "POST") {
    const action = b.action === "resume" ? "resume" : "pause";
    const result = controlMockTask(seg[1], action);
    if (!result.ok) throw new Error(result.error ?? "Task status has changed");
    const task = mockTasks.find((item) => item.id === seg[1]);
    return { id: seg[1], paused: Boolean(task?.paused), queued: Boolean(task?.queued), status: task?.status ?? "" };
  }
  if (seg[0] === "tasks" && seg[2] === "chat" && seg[3] === "status") return { running: false };
  if (seg[0] === "tasks" && seg[2] === "chat" && seg[3] === "stop") return { status: "stopped" };
  if (path === "/active") {
    const id = String(b.id ?? mockActiveTask);
    if (mockTasks.some((item) => item.id === id)) {
      mockActiveTask = id;
      for (const item of mockTasks) item.active = item.id === id;
    }
    return { active: mockActiveTask };
  }

  // ── Coverage / Overlay / Asset association (task dimension)──
  if (seg[0] === "tasks" && seg[2] === "coverage" && seg.length === 3) {
    return seg[1] === "t-northwind" || seg[1] === "5" ? D.northwindCoverage : D.coverage;
  }
  if (seg[0] === "tasks" && seg[2] === "coverage-graph") {
    return seg[1] === "t-northwind" || seg[1] === "5" ? D.northwindCoverageGraph : D.coverageGraph;
  }
  if (seg[0] === "tasks" && seg[2] === "asset-refs") return D.assetRefsFor(Number(q.get("asset_id") ?? 0));

  // ── Task test range (add, delete, check)──
  if (seg[0] === "tasks" && seg[2] === "scope" && seg.length === 3 && m === "GET") {
    return { scope: mockTaskScopes.get(seg[1]) ?? [] };
  }
  if (seg[0] === "tasks" && seg[2] === "goals" && seg.length === 3 && m === "GET") {
    return { goals: seg[1] === "t-acme-web" ? mockGoalCards() : [] };
  }
  if (seg[0] === "tasks" && seg[2] === "constraints" && seg.length === 3 && m === "GET") {
    return { constraints: [] };
  }
  if (seg[0] === "tasks" && seg[2] === "intercept-rules" && seg.length === 3 && m === "GET") {
    const shop = seg[1] === "t-shop-pay" || seg[1] === "3";
    return { rules: shop ? D.interceptRules.filter((rule) => rule.id === 3) : [] };
  }
  if (seg[0] === "tasks" && seg[2] === "scope" && seg.length === 3 && m === "POST") {
    const current = mockTaskScopes.get(seg[1]) ?? [];
    const kind = String(b.kind ?? "") as TaskScopeRow["kind"];
    const value = String(b.value ?? "").trim();
    const row: TaskScopeRow = {
      id: Date.now(),
      task_id: mockTaskAssetID(seg[1]) ?? Number(seg[1]),
      kind,
      source: "manual",
    };
    if (kind === "root_domain" || kind === "subdomain") row.domain = value;
    else if (kind === "ip" || kind === "cidr") row.net = value;
    else row.value = value;
    mockTaskScopes.set(seg[1], [...current, row]);
    return row;
  }
  if (seg[0] === "tasks" && seg[2] === "scope" && seg.length === 4 && m === "DELETE") {
    const id = Number(seg[3]);
    mockTaskScopes.set(
      seg[1],
      (mockTaskScopes.get(seg[1]) ?? []).filter((row) => row.id !== id),
    );
    return { ok: true };
  }

  // ── Global llm_usage Aggregation (new version of dashboard view,demo)──
  if (path === "/tokens/usage") {
    const days = mockRequestedDays(q.get("days"), 365);
    return {
      by_profile: [
        {
          profile_name: D.llmProfiles.find((item) => item.is_default)?.name ?? "default",
          calls: 60,
          tasks: 4,
          input_tokens: 1570000,
          output_tokens: 110000,
          cache_read_tokens: 1120000,
          cache_write_tokens: 140000,
        },
      ],
      daily: mockUsageDaily(days),
    };
  }

  // ── By model token Dosage(demo:An example)──
  if (path === "/llm/records/by-model")
    return {
      models: [
        {
          model: "claude-opus-4-6",
          calls: 42,
          input_tokens: 1250000,
          output_tokens: 86000,
          cache_read_tokens: 940000,
          cache_write_tokens: 120000,
        },
        {
          model: "claude-haiku-4-5",
          calls: 18,
          input_tokens: 320000,
          output_tokens: 24000,
          cache_read_tokens: 180000,
          cache_write_tokens: 20000,
        },
      ],
    };

  // ── Workspace File Manager(demo:Static example tree; write/Build/Delete the details below {ok:true})──
  if (path === "/workspace/list") return D.workspaceList(q.get("path") ?? "");
  if (path === "/workspace/read") return D.workspaceRead(q.get("path") ?? "");

  // ── stats ──
  if (path === "/stats") {
    return D.stats(task, { tasks: mockTasks, findings: mockFindings, activeTask: mockActiveTask });
  }

  // ── assets ──
  if (path === "/assets/counts") return mockAssetCounts(q.get("task_id"));
  if (path === "/assets" && m === "GET") {
    const type = q.get("type") ?? "";
    const taskID = q.get("task_id");
    const numericTaskID = taskID ? mockTaskAssetID(taskID) : undefined;
    const dsl = q.get("dsl") ?? "";
    const list = mockAssets.filter((asset) => {
      if (type && asset.type !== type) return false;
      if (taskID && (numericTaskID === undefined || !asset.task_ids.includes(numericTaskID))) return false;
      return !dsl || mockAssetMatchesDSL(asset, dsl);
    });
    const limit = Number(q.get("limit") ?? 50);
    const offset = Number(q.get("offset") ?? 0);
    const page = list.slice(offset, offset + limit).map((asset) => (taskID ? mockAssetForTask(taskID, asset) : asset));
    return { count: page.length, total: list.length, assets: page };
  }
  if (path === "/assets" && m === "DELETE") {
    const ids = new Set(Array.isArray(b.ids) ? b.ids.map(Number) : []);
    let deleted = 0;
    for (let index = mockAssets.length - 1; index >= 0; index--) {
      if (!ids.has(mockAssets[index].id)) continue;
      mockAssets.splice(index, 1);
      deleted++;
    }
    return { deleted };
  }
  if (seg[0] === "tasks" && seg[2] === "assets" && seg.length === 3 && m === "POST") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    const numericTaskID = mockTaskAssetID(seg[1]);
    if (!task || numericTaskID === undefined) throw new Error("Task does not exist");
    if (Array.isArray(b.scope)) {
      if (b.scope.length === 0) throw new Error("Please fill in the valid test range");
      const rules: CompanyScopeRule[] = b.scope.map((candidate, index) => {
        if (typeof candidate === "string") {
          const issue = classifyCompanyScopeLine(candidate, index + 1);
          if (!issue.rule || issue.error) throw new Error(`Line ${index + 1} has an invalid scope entry: ${issue.error ?? "Unrecognized"}`);
          return issue.rule;
        }
        const item = candidate as { kind?: unknown; value?: unknown };
        const value = String(item?.value ?? "").trim();
        const rule =
          item?.kind && isCompanyScopeKind(item.kind)
            ? { kind: item.kind, value }
            : classifyCompanyScopeLine(value, index + 1).rule;
        const error = rule ? companyScopeRuleError(rule) : "Unrecognized";
        if (!rule || error) throw new Error(`Line ${index + 1} has an invalid scope entry: ${error}`);
        return rule;
      });
      const mutation: TaskAssetScopeMutation = {
        requested: rules.length,
        assets_linked: 0,
        assets_existing: 0,
        scopes_added: 0,
        scopes_existing: 0,
      };
      const currentScopes = mockTaskScopes.get(seg[1]) ?? [];
      const scopeKeys = new Set(
        currentScopes.map((row) => `${row.kind}|${row.domain ?? row.net ?? row.value ?? row.company_id ?? ""}`),
      );
      for (const rule of rules) {
        const normalized = normalizeCompanyScopeValue(rule);
        const scope: TaskScopeRow = {
          id: Date.now() + currentScopes.length,
          task_id: numericTaskID,
          kind: rule.kind === "domain" ? "root_domain" : rule.kind,
          source: "manual",
          reason: "User adds manually on the test asset page",
        };
        if (rule.kind === "domain") scope.domain = normalized;
        else if (rule.kind === "ip") scope.net = `${normalized}/${normalized.includes(":") ? 128 : 32}`;
        else if (rule.kind === "cidr") scope.net = normalized;
        else scope.value = normalized;
        const scopeKey = `${scope.kind}|${scope.domain ?? scope.net ?? scope.value ?? ""}`;
        if (scopeKeys.has(scopeKey)) mutation.scopes_existing++;
        else {
          scopeKeys.add(scopeKey);
          currentScopes.push(scope);
          mutation.scopes_added++;
        }

        if (rule.kind !== "domain" && rule.kind !== "ip") continue;
        const type = rule.kind === "domain" ? "root_domain" : "ip";
        let asset = mockAssets.find((item) =>
          type === "root_domain"
            ? item.type === type && item.domain === normalized
            : item.type === type && item.ip === normalized,
        );
        const alreadyLinked = asset?.task_ids.includes(numericTaskID) ?? false;
        if (!asset) {
          const nextID = mockAssets.reduce((max, item) => Math.max(max, item.id), 0) + 1;
          asset = {
            id: nextID,
            type,
            task_ids: [],
            ...(type === "root_domain" ? { domain: normalized, root_domain: normalized } : { ip: normalized }),
            last_seen: new Date().toISOString(),
          };
          mockAssets.push(asset);
        }
        if (alreadyLinked) mutation.assets_existing++;
        else {
          asset.task_ids.push(numericTaskID);
          mutation.assets_linked++;
        }
        setMockTaskAssetSource(seg[1], asset.id, {
          task_source: "manual",
          task_source_summary: "User adds manually on the test asset page",
          task_source_node_id: undefined,
        });
      }
      mockTaskScopes.set(seg[1], currentScopes);
      return mutation;
    }
    const ids = [...new Set(Array.isArray(b.asset_ids) ? b.asset_ids.map(Number) : [])];
    const sourceSummary = String(b.source_summary ?? "").trim();
    if (ids.length === 0 || ids.length > 100 || !sourceSummary) throw new Error("Please select the asset and fill in the source description");
    const requestedAssets = ids.map((id) => mockAssets.find((asset) => asset.id === id));
    if (requestedAssets.some((asset) => !asset)) throw new Error("Asset does not exist");
    const mutation: TaskAssetMutation = { requested: ids.length, attached: 0, existing: 0 };
    for (const asset of requestedAssets) {
      if (!asset) continue;
      if (asset.task_ids.includes(numericTaskID)) mutation.existing++;
      else {
        asset.task_ids.push(numericTaskID);
        mutation.attached++;
      }
      setMockTaskAssetSource(seg[1], asset.id, {
        task_source: "manual",
        task_source_summary: sourceSummary,
        task_source_node_id: undefined,
      });
    }
    return mutation;
  }
  if (seg[0] === "tasks" && seg[2] === "assets" && seg.length === 4 && m === "DELETE") {
    const numericTaskID = mockTaskAssetID(seg[1]);
    const asset = mockAssets.find((item) => item.id === Number(seg[3]));
    if (numericTaskID === undefined || !asset) throw new Error("Task or asset does not exist");
    if (!asset.task_ids.includes(numericTaskID)) throw new Error("The asset is not associated with the current task");
    asset.task_ids = asset.task_ids.filter((id) => id !== numericTaskID);
    deleteMockTaskAssetSources(seg[1], asset.id);
    return { detached: asset.id };
  }
  if (seg[0] === "tasks" && seg[2] === "intent-assets" && seg.length === 3 && m === "GET") {
    let mappings: Array<{ intentID: string; assetID: number; summary: string }> = [];
    if (seg[1] === "t-acme-web") {
      mappings = [
        { intentID: "i3", assetID: 6, summary: "Backend function enumeration intent selected from preorder subdomain discovery" },
        { intentID: "i5", assetID: 3, summary: "The order interface test intent is selected from the API task target" },
      ];
    } else if (seg[1] === "t-acme-api") {
      mappings = [{ intentID: "i5", assetID: 3, summary: "The order interface test intent is selected from the API task target" }];
    }
    const sourceTaskID = mockTaskAssetID(seg[1]) ?? 0;
    const assets: IntentAsset[] = mappings.flatMap((mapping) => {
      const storedAsset = mockAssets.find((item) => item.id === mapping.assetID);
      if (!storedAsset) return [];
      const asset = mockAssetForTask(seg[1], storedAsset);
      return [
        {
          intent_id: mapping.intentID,
          asset_id: asset.id,
          type: asset.type,
          label: asset.domain ?? asset.ip ?? asset.app_name ?? asset.url ?? asset.service_name ?? `#${asset.id}`,
          source: asset.task_source ?? "agent",
          source_summary: asset.task_source_summary ?? mapping.summary,
          source_node_id: asset.task_source_node_id,
          source_task_id: sourceTaskID,
          inherited: false,
        },
      ];
    });
    return { assets };
  }

  // ── companies ──
  if (path === "/companies" && m === "GET") return structuredClone(mockCompanies);
  if (path === "/companies" && m === "POST") {
    const name = String(b.name ?? "").trim();
    if (!name) throw new Error("Company name cannot be empty");
    if (mockCompanies.some((company) => company.name.toLowerCase() === name.toLowerCase())) {
      throw new Error("The enterprise already exists");
    }
    const id = mockCompanies.reduce((max, company) => Math.max(max, company.id), 0) + 1;
    const scopeResult = mockScopeRows(id, b.scope);
    const company: Company = { id, name, asset_count: 0, scope: scopeResult.rows };
    mockCompanies.push(company);
    return {
      id,
      created: true,
      scope_added: scopeResult.rows.length,
      scope_skipped: scopeResult.skipped,
      scope_invalid: scopeResult.invalid,
    };
  }
  if (seg[0] === "companies" && seg[2] === "scope" && m === "POST") {
    const company = mockCompanies.find((item) => item.id === Number(seg[1]));
    if (!company) throw new Error("The enterprise does not exist");
    const reset = b.reset === true;
    const scopeResult = mockScopeRows(company.id, b.scope, reset ? [] : (company.scope ?? []));
    if (reset && scopeResult.invalid > 0) throw new Error("The enterprise scope contains invalid rules and the original scope is not covered");
    company.scope = reset ? scopeResult.rows : [...(company.scope ?? []), ...scopeResult.rows];
    return { added: scopeResult.rows.length, skipped: scopeResult.skipped, invalid: scopeResult.invalid };
  }
  if (seg[0] === "companies" && seg.length === 2 && m === "DELETE") {
    const id = Number(seg[1]);
    const index = mockCompanies.findIndex((item) => item.id === id);
    if (index < 0) throw new Error("The enterprise does not exist");
    mockCompanies.splice(index, 1);
    let assetsDeleted = 0;
    for (let assetIndex = mockAssets.length - 1; assetIndex >= 0; assetIndex--) {
      const asset: Asset = mockAssets[assetIndex];
      if (asset.company_id !== id) continue;
      if (b.delete_assets === true) {
        mockAssets.splice(assetIndex, 1);
        assetsDeleted++;
      } else {
        delete asset.company_id;
      }
    }
    return { deleted: 1, assets_deleted: assetsDeleted };
  }

  // ── exploration ──
  if (path === "/exploration/frontier") return D.frontier;
  if (path === "/exploration/findings/stats") {
    const vulnclasses = Array.from(new Set(mockFindings.map((f) => f.vulnclass))).sort();
    // [By task]drop down:Vulnerable tasks + Description + Number of items(mock Task id is a string,Directly id Use).
    const taskMap = new Map<string, { name: string; description: string; count: number }>();
    for (const f of mockFindings) {
      if (!f.task_id) continue;
      const owner = mockTasks.find((candidate) => candidate.id === f.task_id);
      const cur = taskMap.get(f.task_id) ?? {
        name: owner?.name ?? "",
        description: f.task_description ?? "",
        count: 0,
      };
      cur.count++;
      taskMap.set(f.task_id, cur);
    }
    const tasks = Array.from(taskMap, ([id, v]) => ({ id, name: v.name, description: v.description, count: v.count }));
    return {
      total: mockFindings.length,
      pending: mockFindings.filter((f) => f.status === "pending").length,
      critical: mockFindings.filter((f) => f.severity === "critical").length,
      high: mockFindings.filter((f) => f.severity === "high").length,
      medium: mockFindings.filter((f) => f.severity === "medium").length,
      low: mockFindings.filter((f) => f.severity === "low").length,
      vulnclasses,
      tasks,
    };
  }
  if (path === "/exploration/findings/asset-tree") {
    const list = mockApplyAssetScope(mockFilterFindings(q), null);
    return { nodes: mockBuildAssetTree(list), finding_total: list.length, truncated: false };
  }
  if (path === "/exploration/findings/groups") {
    const severityOrder = { critical: 4, high: 3, medium: 2, low: 1 } as const;
    const list = mockApplyAssetScope(mockFilterFindings(q), q.get("asset_scope"));

    const grouped = new Map<string, typeof list>();
    for (const finding of list) {
      const key = finding.task_id ?? "__unassigned__";
      grouped.set(key, [...(grouped.get(key) ?? []), finding]);
    }
    const groups = Array.from(grouped, ([key, items]) => {
      const owner = mockTasks.find((candidate) => candidate.id === key);
      return {
        task_id: key === "__unassigned__" ? null : key,
        task_name: owner?.name ?? "",
        task_description: owner?.description ?? items[0]?.task_description ?? "",
        task_status: owner?.status ?? "",
        count: items.length,
        critical: items.filter((finding) => finding.severity === "critical").length,
        high: items.filter((finding) => finding.severity === "high").length,
        medium: items.filter((finding) => finding.severity === "medium").length,
        low: items.filter((finding) => finding.severity === "low").length,
        last_found_at: items.reduce((latest, finding) => (finding.ts > latest ? finding.ts : latest), ""),
        max_severity: Math.max(...items.map((finding) => severityOrder[finding.severity])),
      };
    });
    groups.sort((left, right) =>
      q.get("sort") === "severity"
        ? right.max_severity - left.max_severity || right.last_found_at.localeCompare(left.last_found_at)
        : right.last_found_at.localeCompare(left.last_found_at),
    );
    const rawPage = Number(q.get("page") ?? 1);
    const rawPageSize = Number(q.get("limit") ?? 10);
    const page = Number.isFinite(rawPage) && rawPage > 0 ? Math.floor(rawPage) : 1;
    const pageSize = Number.isFinite(rawPageSize) && rawPageSize > 0 ? Math.min(100, Math.floor(rawPageSize)) : 10;
    return {
      items: groups.slice((page - 1) * pageSize, page * pageSize),
      total: groups.length,
      finding_total: list.length,
      page,
      page_size: pageSize,
    };
  }
  if (path === "/exploration/findings/retests/active" && m === "GET") {
    return {
      retests: mockRetests
        .filter((item) => ["pending", "running"].includes(item.status) && item.conversation_id != null)
        .map((item) => ({
          id: item.id,
          finding_id: D.findings[item.finding_id - 1].id,
          conversation_id: item.conversation_id,
          status: item.status,
        })),
    };
  }
  if (seg[0] === "exploration" && seg[1] === "findings" && seg[3] === "retests") {
    const finding = mockFindings.find((item) => item.id === seg[2]);
    if (!finding) throw new Error("The vulnerability does not exist");
    const findingID = D.findings.findIndex((item) => item.id === finding.id) + 1;
    if (m === "GET") return { retests: structuredClone(mockRetests.filter((item) => item.finding_id === findingID)) };
    if (m === "POST") {
      const existing = mockRetests.find(
        (item) => item.finding_id === findingID && ["pending", "running"].includes(item.status),
      );
      if (existing) return { retest: structuredClone(existing), created: false };
      const now = new Date().toISOString();
      const conversationID = mockConversations.reduce((max, item) => Math.max(max, item.id), 0) + 1;
      mockConversations.unshift({
        id: conversationID,
        agent_key: "retester",
        title: `Retest #${finding.id} · ${finding.name || finding.vulnclass}`,
        pinned: false,
        created_at: now,
        updated_at: now,
      });
      const retest: FindingRetest = {
        id: mockRetests.length + 1,
        finding_id: findingID,
        conversation_id: conversationID,
        status: "running",
        verdict: "",
        notes: String(b.notes ?? ""),
        summary: "",
        evidence: "",
        error: "",
        created_at: now,
        started_at: now,
        finished_at: null,
      };
      mockRetests.unshift(retest);
      mockRetestMessages[conversationID] = [
        {
          seq: 1,
          worker: "retester",
          ts: now,
          kind: "user",
          summary: `Please retest the vulnerability #${finding.id}`,
          detail: retest.notes,
        },
        {
          seq: 2,
          worker: "retester",
          ts: now,
          kind: "text",
          summary: "Demo retest in progress (no request sent to target)",
          detail: "Demo retest in progress (no request sent to target)",
        },
      ];
      return { retest: structuredClone(retest), created: true };
    }
  }
  if (seg[0] === "exploration" && seg[1] === "findings" && seg[3] === "deepen" && m === "POST") {
    const finding = mockFindings.find((candidate) => candidate.id === seg[2]);
    const description = String(b.description ?? "").trim();
    if (!description) throw new Error("description is required");
    if ([...description].length > 4000) throw new Error("description must be at most 4000 characters");
    if (!finding) throw new Error("finding not found");
    if (!finding.task_id || !mockTasks.some((candidate) => candidate.id === finding.task_id)) {
      throw new Error("finding origin task or node is no longer available");
    }
    return {
      task_id: finding.task_id,
      intent_id: `mock-deepen-${Date.now()}`,
      state: "open",
      queued: false,
    };
  }
  // Associated traffic evidence:List / Binding / Edit / Unbind / Sort / Single message details(demo Memory state).
  // seg = ["exploration","findings",<id>,"traffic", ...]
  if (seg[0] === "exploration" && seg[1] === "findings" && seg[3] === "traffic") {
    const findingID = seg[2];
    if (!mockFindings.some((item) => item.id === findingID)) throw new Error("The vulnerability does not exist");
    const bindings = mockTrafficBindings(findingID);
    const bumpVersion = () => {
      mockFindingTrafficVersion[findingID] = (mockFindingTrafficVersion[findingID] ?? 1) + 1;
    };

    // Single message details:GET /traffic/{binding_id}
    if (seg.length === 5 && seg[4] !== "order" && m === "GET") {
      const binding = bindings.find((item) => item.id === seg[4]);
      if (!binding) throw new Error("Evidence does not exist");
      const body = D.trafficBodies[binding.snapshot.source_traffic_id] ?? { req: "", resp: "" };
      return {
        binding: structuredClone(binding),
        request: mockEvidencePreview(body.req),
        response: mockEvidencePreview(body.resp),
      };
    }
    // Message text paging:GET /traffic/{binding_id}/body —— demo The text is not truncated,Return directly to the empty continuation page.
    if (seg.length === 6 && seg[5] === "body" && m === "GET") {
      const binding = bindings.find((item) => item.id === seg[4]);
      if (!binding) throw new Error("Evidence does not exist");
      const body = D.trafficBodies[binding.snapshot.source_traffic_id] ?? { req: "", resp: "" };
      const side = q.get("side") === "request" ? body.req : body.resp;
      return mockEvidencePreview(side);
    }
    // Bind traffic:POST /traffic
    if (seg.length === 4 && m === "POST") {
      const refs = Array.isArray(b.traffic_refs) ? (b.traffic_refs as MockBindingSeed[]) : [];
      for (const ref of refs) {
        if (!ref.traffic_id || bindings.some((item) => item.snapshot.source_traffic_id === ref.traffic_id)) continue;
        bindings.push({
          id: String(++mockBindingSeq),
          finding_id: findingID,
          snapshot_id: `snap-${ref.traffic_id}`,
          role: ref.role ?? "supporting",
          note: ref.note ?? "",
          position: bindings.length,
          created_at: new Date().toISOString(),
          snapshot: mockBuildSnapshot(ref.traffic_id),
        });
      }
      bumpVersion();
      return mockTrafficSummary(findingID);
    }
    // Sort:PUT /traffic/order
    if (seg.length === 5 && seg[4] === "order" && m === "PUT") {
      const order = Array.isArray(b.binding_ids) ? (b.binding_ids as string[]) : [];
      bindings.sort((left, right) => order.indexOf(left.id) - order.indexOf(right.id));
      bumpVersion();
      return mockTrafficSummary(findingID);
    }
    // Editing instructions / Unbind:PATCH|DELETE /traffic/{binding_id}
    if (seg.length === 5 && (m === "PATCH" || m === "DELETE")) {
      const index = bindings.findIndex((item) => item.id === seg[4]);
      if (index < 0) throw new Error("Evidence does not exist");
      if (m === "DELETE") {
        bindings.splice(index, 1);
      } else {
        if (typeof b.role === "string") bindings[index].role = b.role as TrafficEvidenceRole;
        if (typeof b.note === "string") bindings[index].note = b.note;
      }
      bumpVersion();
      return mockTrafficSummary(findingID);
    }
    // List:GET /traffic
    return mockTrafficSummary(findingID);
  }
  if (seg[0] === "exploration" && seg[1] === "findings" && seg[3] === "lineage" && seg.length === 4 && m === "GET") {
    return { nodes: [], edges: [] };
  }
  // Single finding:GET Details / PATCH Change status/Severity/Name/Category(demo Change memory objects directly).
  if (seg[0] === "exploration" && seg[1] === "findings" && seg.length === 3 && seg[2] !== "stats") {
    const f = mockFindings.find((x) => x.id === seg[2]);
    if (!f) return {};
    if (m === "PATCH") {
      if (typeof b.status === "string") f.status = b.status as typeof f.status;
      if (typeof b.severity === "string") f.severity = b.severity as typeof f.severity;
      if (typeof b.name === "string") f.name = b.name;
      if (typeof b.vulnclass === "string") f.vulnclass = b.vulnclass;
    }
    const contextTaskId = q.get("context_task");
    const contextTask = contextTaskId ? mockTasks.find((item) => item.id === contextTaskId) : undefined;
    const inherited = !!(
      contextTask &&
      f.task_id &&
      f.task_id !== contextTask.id &&
      contextTask.source_task_ids?.includes(f.task_id)
    );
    return {
      ...f,
      finding_id: f.id,
      traffic_count: mockTrafficCount(f.id),
      ...(inherited ? { inherited: true, source_task_id: f.task_id } : {}),
    };
  }
  if (path === "/exploration/findings") {
    // finding_id=id:The real backend uses independent table rows id as state/Detail handle,mock use oneself id top.
    // report Only the details interface returns,List Peel(Consistent with backend).
    const withFid = (f: (typeof mockFindings)[number]) => ({
      ...f,
      report: undefined,
      finding_id: f.id,
      traffic_count: mockTrafficCount(f.id),
    });
    if (task) {
      const owner = mockTasks.find((item) => item.id === task);
      const sources = new Set(owner?.source_task_ids ?? []);
      return mockFindings
        .filter((f) => f.task_id === task || (!!f.task_id && sources.has(f.task_id)))
        .map((f) => ({
          ...withFid(f),
          ...(f.task_id !== task ? { inherited: true, source_task_id: f.task_id } : {}),
        }));
    }
    // Global:With page/limit → Paging object;Otherwise bare array(dashboard).
    if (!q.has("page") && !q.has("limit")) return mockFindings.map(withFid);
    const sev = { critical: 4, high: 3, medium: 2, low: 1 } as const;
    const list = mockApplyAssetScope(mockFilterFindings(q), q.get("asset_scope"));
    list.sort((a, b) =>
      q.get("sort") === "severity"
        ? sev[b.severity] - sev[a.severity] || +new Date(b.ts) - +new Date(a.ts)
        : +new Date(b.ts) - +new Date(a.ts),
    );
    const rawPage = Number(q.get("page") ?? 1);
    const rawPageSize = Number(q.get("limit") ?? 20);
    const page = Number.isFinite(rawPage) && rawPage > 0 ? Math.floor(rawPage) : 1;
    const pageSize = Number.isFinite(rawPageSize) && rawPageSize > 0 ? Math.min(200, Math.floor(rawPageSize)) : 20;
    return {
      items: list.slice((page - 1) * pageSize, page * pageSize).map(withFid),
      total: list.length,
      page,
      page_size: pageSize,
    };
  }
  if (path === "/exploration/intents") {
    if (q.has("page")) {
      const before = Number(q.get("before") ?? 0);
      const limit = Math.max(1, Number(q.get("limit") ?? 300));
      let list = mockIntents;
      if (before > 0) list = list.filter((intent) => Number(intent.id.replace(/\D/g, "") || intent.id) < before);
      return { items: list.slice(0, limit), has_more: list.length > limit };
    }
    return mockIntents;
  }
  if (path === "/exploration/tokens") {
    const selectedTask = task ? mockTasks.find((item) => item.id === task) : undefined;
    return {
      workers: D.tokenWorkers,
      sessions: D.tokenSessions,
      total: selectedTask?.tokens ?? D.tokenTotal,
    };
  }
  if (path === "/exploration/graph") return D.explorationGraph;
  // Announcement board:and backend /exploration/nodes Same semantics —— In order of creation(mock Used inside ts + id)Page,
  // And bring the edges involved in this page and the nodes at the other end of the edges.
  if (path === "/exploration/nodes") {
    const all = D.explorationGraph.nodes;
    const kinds = new Set((q.get("kind") ?? "").split(",").filter(Boolean));
    const states = new Set((q.get("state") ?? "").split(",").filter(Boolean));
    const needle = (q.get("q") ?? "").trim().toLowerCase();
    const asc = q.get("order") === "asc";
    const page = Math.max(1, Number(q.get("page") ?? 1));
    const size = Math.min(200, Math.max(1, Number(q.get("size") ?? 20)));
    const rank = (id: string) => all.findIndex((n) => n.id === id);
    const matched = all
      .filter((n) => (kinds.size === 0 || kinds.has(n.type)) && (states.size === 0 || states.has(n.state)))
      .filter(
        (n) =>
          !needle ||
          // Content / source / node id Just hit any one(id Compatible[#41]Writing).
          `${n.payload ?? ""} ${n.origin} ${n.id}`.toLowerCase().includes(needle.replace(/^#/, "")),
      )
      .sort((a, b) => {
        const d = Date.parse(a.ts) - Date.parse(b.ts) || rank(a.id) - rank(b.id);
        return asc ? d : -d;
      });
    const items = matched.slice((page - 1) * size, page * size);
    const onPage = new Set(items.map((n) => n.id));
    const edges = D.explorationGraph.edges.filter((e) => onPage.has(e.src) || onPage.has(e.dst));
    const refs: Record<string, (typeof all)[number]> = {};
    for (const e of edges) {
      for (const id of [e.src, e.dst]) {
        if (onPage.has(id) || refs[id]) continue;
        const node = all.find((n) => n.id === id);
        if (node) refs[id] = node;
      }
    }
    // By the way, bring the node of this page(including neighbors)Anchored assets,For display when expanded.
    const assets: Record<string, ReturnType<typeof D.nodeAssetsFor>> = {};
    for (const id of new Set([...onPage, ...Object.keys(refs)])) {
      const anchored = D.nodeAssetsFor(id);
      if (anchored.length > 0) assets[id] = anchored;
    }
    return { items, total: matched.length, page, size, edges, refs, assets };
  }
  if (path === "/exploration/activity" && seg.length === 2) {
    const since = Number(q.get("since") ?? 0);
    const limit = Math.max(1, Number(q.get("limit") ?? 300));
    const items = mockActivity.filter((item) => item.seq > since).slice(0, limit);
    return { items, cursor: items.length ? items[items.length - 1].seq : since };
  }
  if (seg[0] === "exploration" && seg[1] === "activity" && seg.length === 3) {
    if (seg[2] === "history") return mockActivityHistory(q);
    const a = mockActivity.find((x) => x.seq === Number(seg[2]));
    return { detail: a?.detail ?? a?.summary ?? "" };
  }
  if (path === "/exploration/main-sessions") return mockMainSessions();
  if (path === "/tokens/daily") return mockDailyTokens(mockRequestedDays(q.get("days"), 30));
  if (path === "/tokens/conversations") return D.convTokens;

  // ── traffic / audit / settings ──
  if (path === "/audit") return D.audit;
  if (path === "/traffic" && m === "DELETE") return { deleted: 0 };
  if (path === "/traffic/hosts" && m === "DELETE") return { deleted: (b.hosts as unknown[])?.length ?? 0 };
  if (path === "/traffic/hosts") return { hosts: D.trafficHosts };
  if (path === "/traffic") return mockTrafficList(q);
  if (path === "/traffic/exchange") {
    const body = D.trafficBodies[q.get("id") ?? ""];
    return body ? { req: body.req, resp: body.resp } : { req: "", resp: "" };
  }
  if (path === "/settings" && m === "GET") return D.settings;
  if (path === "/settings" && m === "PUT") return { ...D.settings, ...b };
  if (path === "/settings/web-search/test") return { ok: true, count: 5, backend: D.settings.web_search_backend };
  if (path === "/settings/python/detect") return { python_interpreter: "/usr/bin/python3" };
  if (path === "/chat")
    return { reply: "(demo) I have injected this suggestion as a high-quality intention, and the work agent will execute it as soon as possible.", mode: "hint" };
  if (path === "/gc") return { removed: 0 };

  // ── Tool execution history ──
  if (path === "/commands" && m === "GET") return mockCommandPage(q);
  if (path === "/commands/stats" && m === "GET") {
    const tally = new Map<string, { tool: string; total: number; errors: number }>();
    for (const c of mockFilterCommands(q)) {
      const tool = c.tool || "-";
      const s = tally.get(tool) ?? { tool, total: 0, errors: 0 };
      s.total++;
      if (c.is_error) s.errors++;
      tally.set(tool, s);
    }
    return { stats: [...tally.values()].sort((a, b) => b.total - a.total || a.tool.localeCompare(b.tool)) };
  }

  // ── LLM ──
  if (path === "/llm/records" && m === "GET") return mockLLMRecordPage(q);
  if (path === "/llm/records" && m === "DELETE") return { deleted: 0 };
  if (path === "/llm/records/tasks") {
    const counts = new Map<string, number>();
    for (const record of mockLLMRecords) {
      if (record.task_id) counts.set(record.task_id, (counts.get(record.task_id) ?? 0) + 1);
    }
    return { tasks: [...counts].map(([task_id, count]) => ({ task_id, count })) };
  }
  if (seg[0] === "llm" && seg[1] === "records" && seg.length === 3 && m === "GET") {
    return D.llmRecordDetail(Number(seg[2]), mockLLMRecords);
  }
  if (path === "/llm" && m === "GET") return D.llmConfig;
  if (path === "/llm" && m === "POST") return { ok: true };
  if (path === "/llm/test")
    return { ok: true, latency_ms: 128, model: String(b.model ?? "claude-opus-4-8"), reply: "OK" };
  if (path === "/llm/profiles" && m === "GET") return { profiles: D.llmProfiles };
  if (path === "/llm/profiles" && m === "POST") return { id: Number(b.id) || 3 };
  if (path === "/llm/profiles/active") return { ok: true };
  if (path === "/llm/pool" && m === "GET") return D.llmPool;
  if (path === "/llm/retry-policy" && m === "GET") {
    return {
      connect: mockRetryRule,
      empty: mockRetryRule,
      stream: mockRetryRule,
      breaker: mockRetryRule,
      intent: mockRetryRule,
    };
  }
  if (path === "/llm/pool/reset")
    return { ...D.llmPool, chain: D.llmPool.chain.map((c) => ({ ...c, state: "ok", fails: 0, cooldown_secs: 0 })) };
  if (seg[0] === "llm" && seg[1] === "profiles" && seg.length === 3 && m === "DELETE")
    return { deleted: Number(seg[2]) };

  // ── agents ──
  if (path === "/agents" && m === "GET") return { agents: D.agents };
  if (path === "/agents" && m === "POST")
    return {
      id: "9",
      key: String(b.key ?? "custom"),
      name: String(b.name ?? ""),
      role: "custom",
      builtin: false,
      enabled: true,
    };
  if (seg[0] === "agents" && seg.length === 2 && m === "GET") return D.agentDetail(seg[1]);
  if (seg[0] === "agents" && seg[2] === "triggers" && m === "GET") return { triggers: [] };
  if (seg[0] === "agents" && seg[2] === "prompts") return { versions: D.agentDetail(seg[1]).versions };
  if (seg[0] === "agents" && seg[2] === "variables") return { variables: D.agentDetail(seg[1]).variables };
  if (seg[0] === "agents" && seg[2] === "prompt" && seg[3] === "preview")
    return { rendered: String(b.template ?? "").replace(/\{\{\.(\w+)\}\}/g, "«$1»") };
  if (seg[0] === "agents" && seg[2] === "visibility" && m === "GET") return D.agentDetail(seg[1]).visibility;

  // ── conversations ──
  if (path === "/conversations" && m === "GET") {
    sortMockConversations();
    return {
      conversations: structuredClone(
        mockConversations.map((conversation) => ({
          ...conversation,
          running: mockRetests.some(
            (item) => item.conversation_id === conversation.id && ["pending", "running"].includes(item.status),
          ),
        })),
      ),
    };
  }
  if (path === "/conversations" && m === "POST") {
    const now = new Date().toISOString();
    const title = String(b.title ?? "").trim() || "New conversation";
    const conversation: Conversation = {
      id: mockConversations.reduce((max, item) => Math.max(max, item.id), 0) + 1,
      agent_key: String(b.agent_key ?? "mainagent"),
      title,
      llm_profile_id: typeof b.llm_profile_id === "number" ? b.llm_profile_id : undefined,
      pinned: false,
      created_at: now,
      updated_at: now,
    };
    mockConversations.unshift(conversation);
    return structuredClone(conversation);
  }
  if (seg[0] === "conversations" && seg.length === 2 && m === "PATCH") {
    const conversation = mockConversations.find((item) => item.id === Number(seg[1]));
    if (!conversation) return {};
    if (typeof b.title === "string") conversation.title = b.title.trim();
    if (typeof b.pinned === "boolean") {
      conversation.pinned = b.pinned;
      conversation.pinned_at = b.pinned ? (conversation.pinned_at ?? new Date().toISOString()) : null;
    }
    conversation.updated_at = new Date().toISOString();
    sortMockConversations();
    return structuredClone(conversation);
  }
  if (seg[0] === "conversations" && seg.length === 2 && m === "DELETE") {
    const id = Number(seg[1]);
    stopMockRetest(id);
    const index = mockConversations.findIndex((item) => item.id === id);
    if (index >= 0) mockConversations.splice(index, 1);
    for (const retest of mockRetests) if (retest.conversation_id === id) retest.conversation_id = null;
    return { deleted: id };
  }
  if (path === "/conversations/delete/batch" && m === "POST") {
    const ids = Array.isArray(b.ids)
      ? [...new Set(b.ids.map(Number).filter((id) => Number.isInteger(id) && id > 0))]
      : [];
    const items = ids.map((id) => {
      const index = mockConversations.findIndex((item) => item.id === id);
      if (index < 0) return { id, ok: false, error: "conversation not found" };
      mockConversations.splice(index, 1);
      stopMockRetest(id);
      for (const retest of mockRetests) if (retest.conversation_id === id) retest.conversation_id = null;
      return { id, ok: true };
    });
    return { items };
  }
  if (seg[0] === "conversations" && seg[2] === "messages" && seg.length === 3 && m === "GET") {
    const items = mockRetestMessages[Number(seg[1])] ?? D.conversationMessages[Number(seg[1])] ?? [];
    const running = mockRetests.some(
      (item) => item.conversation_id === Number(seg[1]) && ["pending", "running"].includes(item.status),
    );
    return { items, cursor: items.length ? items[items.length - 1].seq : 0, running };
  }
  if (seg[0] === "conversations" && seg[2] === "messages" && seg.length === 4) {
    const msgs = mockRetestMessages[Number(seg[1])] ?? D.conversationMessages[Number(seg[1])] ?? [];
    const a = msgs.find((x) => x.seq === Number(seg[3]));
    return { detail: a?.detail ?? a?.summary ?? "" };
  }
  if (seg[0] === "conversations" && seg[2] === "messages" && m === "POST") return { status: "ok" };
  if (seg[0] === "conversations" && seg[2] === "stop") {
    stopMockRetest(Number(seg[1]));
    return { status: "stopped" };
  }

  // ── tools ──
  if (path === "/tools" && m === "GET") return { tools: D.tools };
  if (path === "/tools/custom" && m === "POST") return { key: String(b.key ?? "custom-tool") };
  if (path === "/tools/custom/test") return { output: "(demo) Tool execution output example.", is_error: false };

  // ── mcp ──
  if (path === "/mcp" && m === "GET") return { servers: D.mcpServers };
  if (path === "/mcp" && m === "POST") return { id: 3 };
  if (seg[0] === "mcp" && seg[2] === "tools") return { tools: D.mcpToolsById[Number(seg[1])] ?? [] };
  if (seg[0] === "mcp" && seg[2] === "refresh") return { tools: D.mcpToolsById[Number(seg[1])] ?? [] };
  if (seg[0] === "mcp" && seg.length === 2 && m === "DELETE") return { deleted: Number(seg[1]) };

  // ── scopesentry(demo:Not configured)──
  if (path === "/sync/scopesentry/status")
    return { exists: false, configured: false, enabled: false, reachable: false, tools: [] };
  if (path === "/sync/scopesentry/projects") return { projects: [], tag: {} };
  if (path === "/sync/scopesentry/tasks") return { tasks: [] };
  if (path === "/sync/scopesentry/sync") return { synced: {}, companies: null, warnings: null, errors: null };

  // ── skills ──
  if (path === "/skills" && m === "GET") return { skills: D.skills };
  if (path === "/skills/missing") return { missing: D.missingSkills };
  if (seg[0] === "skills" && seg[2] === "usage") return { calls: D.skillCalls };
  if (path === "/skills" && m === "POST") return { name: String(b.name ?? "new-skill") };
  if (seg[0] === "skills" && seg[2] === "files" && seg.length === 3) return { files: ["SKILL.md"] };
  if (seg[0] === "skills" && seg[2] === "files" && seg.length >= 4)
    return { content: "#SKILL.md\n\n(demo) This is a sample documentation for this skill.", file: seg.slice(3).join("/") };

  // ── visibility ──
  if (seg[0] === "visibility" && m === "GET") return { agents: [] };

  // ── intercept ──
  if (path === "/intercept/rules" && m === "GET") return { rules: D.interceptRules };
  if (path === "/asset-intercept/rules" && m === "GET") return { rules: [] };
  if (seg[0] === "intercept" && seg[1] === "rules" && seg[3] === "toggle")
    return { ok: true, enabled: b.enabled ?? true };
  if (path === "/intercept/pending" && m === "GET")
    return { pending: mockInterceptHistory.filter((r) => r.status === "pending") };
  if (seg[0] === "intercept" && seg[1] === "pending" && seg[3] === "decide") {
    const id = Number(seg[2]);
    const row = mockInterceptHistory.find((r) => r.id === id) ?? mockInterceptPending.find((r) => r.id === id);
    if (row?.status !== "pending") throw new Error("The approval has been processed or does not exist, please refresh the record");
    if (b.decision !== "allowed" && b.decision !== "denied") throw new Error("Invalid approval action");
    row.status = b.decision;
    row.decided_at = new Date().toISOString();
    const detail = mockInterceptDetails[id];
    if (detail) {
      detail.effective_action = b.decision === "allowed" ? "allow" : "deny";
      detail.decision_reason = b.decision === "allowed" ? "Manually allowed execution" : "Manual refusal to execute";
      detail.execution_status = b.decision === "allowed" ? "unknown" : "not_executed";
      detail.output = b.decision === "allowed" ? "Tool not executed in demo mode." : "";
    }
    return { ok: true };
  }
  if (seg[0] === "intercept" && seg[1] === "history" && seg[3] === "execution" && seg.length === 4 && m === "GET") {
    const row = mockInterceptHistory.find((item) => item.id === Number(seg[2]));
    return {
      conversation_id: row?.conversation_id ?? null,
      task_id: row?.task_id ?? null,
      session: row?.task_id ? "plan" : "main",
      seq: 0,
      items: [],
    };
  }
  if (seg[0] === "intercept" && seg[1] === "history" && seg.length === 3) {
    const row = mockInterceptHistory.find((r) => r.id === Number(seg[2]));
    if (!row) throw new Error("The approval record does not exist");
    return { ...row, audit: mockInterceptDetails[row.id] ?? null };
  }
  if (seg[0] === "intercept" && seg[1] === "pending" && seg.length === 3 && m === "GET")
    return (
      mockInterceptPending.find((p) => p.id === Number(seg[2])) ??
      mockInterceptHistory.find((p) => p.id === Number(seg[2])) ??
      null
    );
  if (path === "/intercept/history" || (seg[0] === "intercept" && seg[1] === "task")) {
    const status = q.get("status") || "";
    const decisionSource = q.get("decision_source") || "";
    if (status && !["pending", "allowed", "denied", "timeout"].includes(status)) throw new Error("Invalid approval status");
    if (decisionSource && !["model", "rule", "unknown"].includes(decisionSource)) throw new Error("Invalid determination source");
    const filtered = mockInterceptHistory.filter((row) => {
      const source =
        row.decision_source || (row.rule_id ? "rule" : row.reason?.startsWith("[Model]") ? "model" : "unknown");
      return (
        (seg[1] !== "task" || row.task_id === decodeURIComponent(seg[2])) &&
        (!status || row.status === status) &&
        (!decisionSource || source === decisionSource)
      );
    });
    if (!q.has("page") && !q.has("size") && !status && !decisionSource)
      return { items: filtered, total: filtered.length };
    const page = Math.max(1, Number(q.get("page")) || 1);
    const size = Math.min(100, Math.max(1, Number(q.get("size")) || 20));
    const offset = (page - 1) * size;
    return {
      items: filtered.slice(offset, offset + size),
      total: filtered.length,
      page,
      page_size: size,
    };
  }
  if (path === "/intercept/tool-config") return { enabled_tools: ["bash"] };
  if (path === "/intercept/judge" && m === "GET")
    return {
      enabled: true,
      profile_id: 1,
      prompt: "These figures are sample data. This preview did not call a model.",
      timeout_seconds: 15,
      fail_action: "ask",
      ask_timeout_seconds: 300,
      ask_timeout_action: "deny",
    };
  if (path === "/intercept/judge" && m === "PUT") return { ok: true };
  if (path === "/intercept/judge/usage" && m === "GET") return mockJudgeUsage(mockRequestedDays(q.get("days"), 30));

  if (path === "/notify/meta" && m === "GET")
    return {
      ...mockNotifyMeta,
      stats: {
        channels: D.notifyChannels.length,
        channels_on: D.notifyChannels.filter((channel) => channel.enabled).length,
        pending: 0,
        failed: D.notifyDeliveries.filter((row) => row.state === "failed").length,
        sent_today: D.notifyDeliveries.filter((row) => row.state === "sent").length,
        backlog_age_ms: 0,
      },
    };
  if (path === "/notify/channels" && m === "GET") return { channels: D.notifyChannels };
  if (path === "/notify/deliveries" && m === "GET") return mockNotifyDeliveries(q);
  if (seg[0] === "notify" && seg[1] === "channels" && seg[3] === "test" && m === "POST") {
    throw new Error("preview endpoint refused the connection");
  }
  if (path === "/update/check" && m === "GET") {
    return {
      current: "dev",
      mode: "binary",
      os: "linux",
      arch: "x64",
      repo: "skilfoy/ARTEX",
      has_backup: false,
      comparable: false,
      has_update: false,
      reason: "Demo build is not compared with a release.",
    };
  }
  if (path === "/logs/history" && m === "GET") return mockCampaignLogs(q);

  // ── Bypass question(/btw):demo No bypass session ──
  // Must explicitly hit: path ends with s The ending will be judged as a set and returned by the following reading [],items That's it undefined.
  if (seg.at(-1) === "side-questions") {
    if (m === "GET") return { items: [], current: null, next_cursor: 0, snapshot: null };
    if (m === "POST") throw new Error("Demo mode does not support side questions");
  }

  // ── The write operation is complete: successful but not lost in the database ──
  if (["POST", "PUT", "PATCH", "DELETE"].includes(m)) return { ok: true };

  // Unmatched reads throw only in the mock route test. Production preview keeps an empty body.
  if (m === "GET" && process.env.MOCK_STRICT === "1") {
    throw new Error(`unhandled GET ${path}`);
  }

  // ── Read the ins and outs: collection classes [],The rest {} ──
  return /(\/(tasks|profiles|conversations|rules|history|projects|tokens|agents|servers|skills|tools|findings|intents)s?$)|s$/.test(
    path,
  )
    ? []
    : {};
}
