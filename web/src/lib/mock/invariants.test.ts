import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { activity, dailyTokens, findings, llmRecords, tasks, tokenTotal, traffic } from "./data";

const DAY = 24 * 60 * 60 * 1000;
const now = Date.now();
const taskIDs = new Set(tasks.map((task) => task.id));

test("finding counts on each task match the finding rows", () => {
  for (const task of tasks) {
    const rows = findings.filter((finding) => finding.task_id === task.id);
    const counts = { critical: 0, high: 0, medium: 0, low: 0 };
    for (const finding of rows) {
      const severity = finding.severity as keyof typeof counts;
      assert.ok(severity in counts, `${finding.id} has unknown severity ${finding.severity}`);
      counts[severity] += 1;
    }
    assert.deepEqual(task.findings, counts, task.id);
    assert.ok((task.source_task_ids ?? []).every((id) => taskIDs.has(id)), task.id);
  }
});

test("daily token series covers every dashboard range", () => {
  assert.equal(dailyTokens.length, 400);
  const newest = dailyTokens.at(-1);
  assert.ok(newest);
  assert.equal(newest.date, new Date().toISOString().slice(0, 10));
  for (const bucket of dailyTokens) {
    const age = now - Date.parse(`${bucket.date}T00:00:00Z`);
    assert.ok(age < 400 * DAY, bucket.date);
    assert.ok(bucket.input_tokens > 0);
  }
});

test("fixture timestamps stay inside the current window", () => {
  const stamps = [
    ...tasks.flatMap((task) => [task.created_at, task.last_activity, task.completed_at]),
    ...findings.map((finding) => finding.ts),
    ...(traffic.exchanges ?? []).map((exchange) => exchange.ts),
    ...activity.map((item) => item.ts),
  ].filter((value): value is string => Boolean(value));
  for (const stamp of stamps) {
    const age = now - Date.parse(stamp);
    assert.ok(age >= 0, `future ${stamp}`);
    assert.ok(age < 400 * DAY, `too old ${stamp}`);
  }
  const running = tasks.filter((task) => task.status === "running");
  for (const task of running) {
    assert.ok(task.created_at && now - Date.parse(task.created_at) < 14 * DAY, task.id);
    assert.ok(task.last_activity && now - Date.parse(task.last_activity) < 60 * 60 * 1000, task.id);
  }
});

test("linked ids exist and retired strings are gone", () => {
  for (const finding of findings) assert.ok(finding.task_id && taskIDs.has(finding.task_id), finding.id);
  for (const record of llmRecords) assert.ok(record.task_id && taskIDs.has(record.task_id), String(record.id));
  assert.equal(
    tokenTotal.input_tokens,
    tasks.reduce((sum, task) => sum + (task.tokens?.input_tokens ?? 0), 0),
  );
  const source = readFileSync(new URL("./data.ts", import.meta.url), "utf8");
  for (const banned of ["t-001", "Chaoyang", "138****6021"]) {
    assert.equal(source.includes(banned), false, banned);
  }
});
