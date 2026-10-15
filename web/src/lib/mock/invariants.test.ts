import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { activity, assets, companies, dailyTokens, findingRetestSeeds, findingTrafficSeeds, findings, llmRecords, tasks, tokenTotal, traffic, trafficBodies } from "./data";

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
  for (const banned of ["t-001", "Chaoyang", "138****6021", "Vulnerability Overview"]) {
    assert.equal(source.includes(banned), false, banned);
  }
});

test("every finding has a dossier and its own traffic", () => {
  const exchangeIDs = new Set((traffic.exchanges ?? []).map((exchange) => exchange.id));
  assert.equal(findings.length, Object.keys(findingTrafficSeeds).length);
  for (const finding of findings) {
    assert.match(finding.report ?? "", /## What was observed/);
    assert.match(finding.report ?? "", /## Asset/);
    assert.match(finding.report ?? "", /## Why this severity/);
    assert.match(finding.report ?? "", /## What to verify next/);
    assert.doesNotMatch(finding.report ?? "", /sqlmap|jndi|scriptText|psexec/i, finding.id);
    const seeds = findingTrafficSeeds[finding.id];
    assert.ok(seeds && seeds.length > 0, finding.id);
    for (const seed of seeds) {
      assert.equal(exchangeIDs.has(seed.traffic_id), true, `${finding.id} ${seed.traffic_id}`);
      assert.doesNotMatch(seed.note, /sqlmap|jndi|scriptText|psexec/i, finding.id);
      const body = trafficBodies[seed.traffic_id];
      const exchange = (traffic.exchanges ?? []).find((item) => item.id === seed.traffic_id);
      assert.ok(body && exchange);
      const path = new URL(exchange.url).pathname + new URL(exchange.url).search;
      assert.ok(body.req.includes(path), seed.traffic_id);
      assert.ok(body.req.includes(exchange.host), seed.traffic_id);
      assert.match(body.resp, new RegExp(`HTTP/1\\.1 ${exchange.status}`));
    }
  }
  assert.deepEqual(
    findingRetestSeeds.map((seed) => [seed.finding_id, seed.verdict]),
    [
      ["f-1", "reproduced"],
      ["f-4", "fixed"],
      ["f-6", "inconclusive"],
    ],
  );
});

test("Northwind is a finished engagement and does not reuse Acme hosts", () => {
  assert.equal(tasks[4]?.id, "t-northwind");
  const task = tasks[4];
  assert.equal(task?.status, "done");
  assert.deepEqual(task?.company_ids, [2]);
  assert.deepEqual(task?.findings, { critical: 1, high: 1, medium: 1, low: 0 });
  const company = companies.find((item) => item.id === 2);
  assert.equal(company?.name, "Northwind Trading");
  const rows = assets.filter((asset) => asset.company_id === 2);
  assert.equal(rows.length, company?.asset_count);
  assert.deepEqual(
    rows.map((asset) => asset.id),
    [27, 28, 29, 30],
  );
  for (const asset of rows) {
    const blob = `${asset.domain ?? ""} ${asset.ip ?? ""} ${asset.url ?? ""}`;
    assert.equal(blob.includes("acme.com"), false, blob);
    assert.deepEqual(asset.task_ids, [5]);
  }
  const northwindFindings = findings.filter((finding) => finding.task_id === "t-northwind");
  assert.deepEqual(
    northwindFindings.map((finding) => finding.id),
    ["f-n1", "f-n2", "f-n3"],
  );
  for (const finding of northwindFindings) {
    assert.equal(finding.assets?.length, 1, finding.id);
    assert.equal(finding.assets?.[0]?.label.includes("acme.com"), false, finding.id);
    assert.equal(finding.assets?.[0]?.label.includes("northwind"), true, finding.id);
  }
  const northwindTraffic = (traffic.exchanges ?? []).filter((exchange) => exchange.id.startsWith("x-n"));
  assert.deepEqual(
    northwindTraffic.map((exchange) => exchange.id),
    ["x-n1", "x-n2", "x-n3"],
  );
  for (const exchange of northwindTraffic) {
    assert.equal(exchange.host.includes("acme.com"), false, exchange.id);
    assert.equal(exchange.host.includes("northwind") || exchange.host.startsWith("198.51.100."), true, exchange.host);
  }
});
