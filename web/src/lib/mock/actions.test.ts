import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { PREVIEW_UNAVAILABLE, mockHandle } from "./handler";

function isBareOk(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const keys = Object.keys(value as object);
  return keys.length === 1 && keys[0] === "ok" && (value as { ok?: unknown }).ok === true;
}

function sample(expr: string): string {
  const e = expr.trim();
  if (e.startsWith("tq(") || e.includes("?")) return "";
  if (/task/i.test(e)) return "t-acme-web";
  if (/skill/i.test(e) && /file|path/i.test(e)) return "SKILL.md";
  if (/skill/i.test(e)) return "api-recon";
  if (/agent|key/i.test(e)) return "planner";
  if (/intent/i.test(e)) return "i1";
  if (/finding/i.test(e)) return "f-1";
  if (/binding/i.test(e)) return "b-f-1-1";
  if (/file|path/i.test(e)) return "notes.md";
  return "1";
}

function fillPath(raw: string): string {
  return raw.replace(/\$\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}/g, (_, expr: string) => sample(expr));
}

function clientWrites(source: string): { method: string; path: string }[] {
  const found: { method: string; path: string }[] = [];
  const call = /\b(post|put|patch|del)\s*(?:<[\s\S]*?>)?\s*\(\s*`([^`]+)`/g;
  for (const match of source.matchAll(call)) {
    const method = { post: "POST", put: "PUT", patch: "PATCH", del: "DELETE" }[match[1]] ?? "POST";
    const path = fillPath(match[2]).split("${").shift() ?? "";
    if (path.startsWith("/")) found.push({ method, path });
  }
  const http = /http\s*(?:<[\s\S]*?>)?\s*\(\s*`([^`]+)`\s*,\s*\{[\s\S]*?method:\s*"(POST|PUT|PATCH|DELETE)"/g;
  for (const match of source.matchAll(http)) {
    const path = fillPath(match[1]).split("${").shift() ?? "";
    if (path.startsWith("/")) found.push({ method: match[2], path });
  }
  return found;
}

test("pause, chat, approval, and template create change the next read", { timeout: 60_000 }, async () => {
  const beforeFindings = await mockHandle<{ finding_id: string }[]>("GET", "/exploration/findings");
  const beforeIds = beforeFindings.map((row) => row.finding_id).sort();

  const paused = await mockHandle<{ status: string; paused: boolean; engine_mode?: string }>(
    "POST",
    "/tasks/t-acme-web/control",
    JSON.stringify({ action: "pause" }),
  );
  assert.equal(paused.status, "paused");
  const pausedList = await mockHandle<{ tasks: { id: string; status: string; paused: boolean; engine_mode: string }[] }>(
    "GET",
    "/tasks",
  );
  const pausedRow = pausedList.tasks.find((task) => task.id === "t-acme-web");
  assert.equal(pausedRow?.status, "paused");
  assert.equal(pausedRow?.paused, true);
  assert.equal(pausedRow?.engine_mode, "paused");
  const pausedDetail = await mockHandle<{ status: string; paused: boolean; engine_mode: string }>(
    "GET",
    "/tasks/t-acme-web",
  );
  assert.equal(pausedDetail.status, "paused");
  assert.equal(pausedDetail.paused, true);
  assert.equal(pausedDetail.engine_mode, "paused");

  const resumed = await mockHandle<{ status: string }>("POST", "/tasks/t-acme-web/control", JSON.stringify({ action: "resume" }));
  assert.equal(resumed.status, "running");
  const resumedDetail = await mockHandle<{ status: string; paused: boolean; engine_mode: string }>(
    "GET",
    "/tasks/t-acme-web",
  );
  assert.equal(resumedDetail.status, "running");
  assert.equal(resumedDetail.paused, false);
  assert.equal(resumedDetail.engine_mode, "exploring");

  const message = "Where should the next check start?";
  const chat = await mockHandle<{ reply: string; mode: string }>(
    "POST",
    "/chat?task=t-acme-web",
    JSON.stringify({ message, seg: 0 }),
  );
  assert.match(chat.reply, /Acme web assessment/);
  assert.match(chat.reply, /not an agent run/i);
  assert.match(chat.reply, /does not add a finding/i);
  assert.notEqual(chat.mode, "llm");
  const activity = await mockHandle<{ items: { summary: string; kind: string }[] }>(
    "GET",
    "/exploration/activity?task=t-acme-web",
  );
  assert.ok(activity.items.some((item) => item.kind === "user" && item.summary === message));
  assert.ok(activity.items.some((item) => item.summary === chat.reply));
  const history = await mockHandle<{ items: { summary: string }[] }>(
    "GET",
    "/exploration/activity/history?session=main:0&task=t-acme-web",
  );
  assert.ok(history.items.some((item) => item.summary === message));
  assert.ok(history.items.some((item) => item.summary === chat.reply));
  const afterFindings = await mockHandle<{ finding_id: string }[]>("GET", "/exploration/findings");
  assert.deepEqual(
    afterFindings.map((row) => row.finding_id).sort(),
    beforeIds,
  );

  const pendingBefore = await mockHandle<{ pending: { id: number; status: string }[] }>("GET", "/intercept/pending");
  assert.equal(pendingBefore.pending.length, 1);
  assert.equal(pendingBefore.pending[0]?.id, 101);
  const decision = await mockHandle<{ ok: boolean; status: string; decided_at: string }>(
    "POST",
    "/intercept/pending/101/decide",
    JSON.stringify({ decision: "denied" }),
  );
  assert.equal(decision.ok, true);
  assert.equal(decision.status, "denied");
  assert.ok(decision.decided_at);
  const pendingAfter = await mockHandle<{ pending: { id: number }[] }>("GET", "/intercept/pending");
  assert.equal(pendingAfter.pending.length, 0);
  const approvals = await mockHandle<{ items: { id: number; status: string; decided_at?: string }[] }>(
    "GET",
    "/intercept/history",
  );
  const decided = approvals.items.find((row) => row.id === 101);
  assert.equal(decided?.status, "denied");
  assert.equal(decided?.decided_at, decision.decided_at);

  const templates = await mockHandle<{ templates: { name: string; description: string; goal: string }[] }>(
    "GET",
    "/task-templates",
  );
  const template = templates.templates[0];
  assert.ok(template);
  const created = await mockHandle<{
    id: string;
    status: string;
    name: string;
    findings: { critical: number; high: number; medium: number; low: number };
  }>("POST", "/tasks", JSON.stringify({ name: template.name, description: template.description, goal: template.goal }));
  assert.match(created.id, /^t-new-/);
  assert.equal(created.status, "running");
  assert.equal(created.name, template.name);
  assert.deepEqual(created.findings, { critical: 0, high: 0, medium: 0, low: 0 });
  const tasks = await mockHandle<{ tasks: { id: string; status: string }[] }>("GET", "/tasks");
  assert.equal(tasks.tasks.find((task) => task.id === created.id)?.status, "running");
  const createdFindings = await mockHandle<unknown[]>("GET", `/exploration/findings?task=${created.id}`);
  assert.deepEqual(createdFindings, []);

  const sideBefore = await mockHandle<{ items: unknown[] }>("GET", "/tasks/t-acme-web/chat/side-questions");
  assert.deepEqual(sideBefore.items, []);
  await assert.rejects(
    () => mockHandle("POST", "/tasks/t-acme-web/chat/side-questions", JSON.stringify({ question: "What changed?" })),
    (error: Error) => error.message === PREVIEW_UNAVAILABLE,
  );
  const sideAfter = await mockHandle<{ items: unknown[] }>("GET", "/tasks/t-acme-web/chat/side-questions");
  assert.deepEqual(sideAfter.items, []);
  await assert.rejects(
    () => mockHandle("PUT", "/settings", JSON.stringify({ notify_enabled: false })),
    (error: Error) => error.message === PREVIEW_UNAVAILABLE,
  );
  const settings = await mockHandle<{ notify_enabled?: boolean }>("GET", "/settings");
  assert.notEqual(settings.notify_enabled, false);
});

test("a UI write cannot receive a bare ok response", { timeout: 120_000 }, async () => {
  const handlerSrc = readFileSync(new URL("./handler.ts", import.meta.url), "utf8");
  const apiSrc = readFileSync(new URL("../api.ts", import.meta.url), "utf8");
  assert.equal(handlerSrc.match(/return\s*\{\s*ok:\s*true\s*\}/g), null);
  assert.equal(apiSrc.match(/if \(MOCK\) return;/g), null);
  assert.ok(apiSrc.split("throw new Error(PREVIEW_UNAVAILABLE)").length >= 5);

  const writes = clientWrites(apiSrc);
  assert.ok(writes.length > 40, `expected the client write inventory, got ${writes.length}`);
  const bare: string[] = [];
  for (const write of writes) {
    try {
      const result = await mockHandle(write.method, write.path, JSON.stringify({}));
      if (isBareOk(result)) bare.push(`${write.method} ${write.path}`);
    } catch {
      // A thrown error is the supported answer for a write this preview will not apply.
    }
  }
  assert.deepEqual(bare, []);
});
