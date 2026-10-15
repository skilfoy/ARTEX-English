import assert from "node:assert/strict";
import test from "node:test";
import { mockHandle } from "./handler";

test("each traffic row has its own body and filters stay exact", async () => {
  const all = await mockHandle<{ count: number; total: number; exchanges: { id: string; host: string }[] }>(
    "GET",
    "/traffic?page=0&size=50",
  );
  assert.equal(all.count, 22);
  assert.equal(all.total, 22);
  assert.equal(all.exchanges.length, 22);

  const filtered = await mockHandle<{ count: number; total: number; exchanges: { host: string }[] }>(
    "GET",
    "/traffic?host=www.northwind.example&method=GET&page=0&size=50",
  );
  assert.equal(filtered.count, 22);
  assert.equal(filtered.total, 2);
  assert.deepEqual(
    filtered.exchanges.map((row) => row.host),
    ["www.northwind.example", "www.northwind.example"],
  );

  const missing = await mockHandle<{ req: string; resp: string }>("GET", "/traffic/exchange?id=does-not-exist");
  assert.deepEqual(missing, { req: "", resp: "" });

  const home = await mockHandle<{ req: string; resp: string }>("GET", "/traffic/exchange?id=x-1");
  assert.match(home.req, /GET \/ HTTP\/1\.1/);
  assert.match(home.req, /Host: www\.acme\.com/);
  assert.match(home.resp, /HTTP\/1\.1 200/);
  assert.equal(home.resp.includes("order_id"), false);

  const admin = await mockHandle<{ req: string; resp: string }>("GET", "/traffic/exchange?id=x-n1");
  assert.match(admin.req, /GET \/admin HTTP\/1\.1/);
  assert.match(admin.req, /Host: www\.northwind\.example/);
  assert.equal(admin.resp.includes("order_id"), false);
});

test("retests are finished and do not leak onto other findings", async () => {
  const active = await mockHandle<{ retests: unknown[] }>("GET", "/exploration/findings/retests/active");
  assert.deepEqual(active.retests, []);

  const reproduced = await mockHandle<{ retests: { verdict: string; status: string; finding_id: number }[] }>(
    "GET",
    "/exploration/findings/f-1/retests",
  );
  assert.equal(reproduced.retests.length, 1);
  assert.equal(reproduced.retests[0]?.status, "completed");
  assert.equal(reproduced.retests[0]?.verdict, "reproduced");

  const fixed = await mockHandle<{ retests: { verdict: string }[] }>("GET", "/exploration/findings/f-4/retests");
  assert.equal(fixed.retests[0]?.verdict, "fixed");
  const inconclusive = await mockHandle<{ retests: { verdict: string }[] }>(
    "GET",
    "/exploration/findings/f-6/retests",
  );
  assert.equal(inconclusive.retests[0]?.verdict, "inconclusive");

  for (const id of ["f-2", "f-12", "f-17", "f-n1", "f-n2", "f-n3"]) {
    const other = await mockHandle<{ retests: unknown[] }>("GET", `/exploration/findings/${id}/retests`);
    assert.equal(other.retests.length, 0, id);
  }
});

test("Northwind coverage and archive stay separate from Acme", async () => {
  const acme = await mockHandle<{ nodes: { label: string }[] }>("GET", "/tasks/t-acme-web/coverage-graph");
  assert.equal(
    acme.nodes.some((node) => node.label === "Acme Corp"),
    true,
  );
  assert.equal(
    acme.nodes.some((node) => node.label.toLowerCase().includes("northwind")),
    false,
  );

  const northwind = await mockHandle<{ nodes: { label: string; tested?: boolean }[] }>(
    "GET",
    "/tasks/t-northwind/coverage-graph",
  );
  assert.deepEqual(
    northwind.nodes.map((node) => node.label),
    ["Northwind Trading", "northwind.example", "www.northwind.example", "mail.northwind.example"],
  );
  assert.equal(northwind.nodes.filter((node) => node.tested).length, 1);
  assert.equal(northwind.nodes.find((node) => node.tested)?.label, "www.northwind.example");

  const coverage = await mockHandle<{ denominator: number; tested: number }>("GET", "/tasks/t-northwind/coverage");
  assert.equal(coverage.denominator, 4);
  assert.equal(coverage.tested, 1);

  const archives = await mockHandle<{
    total: number;
    items: { id: number; task_name: string; original_status: string; archived_at?: string; data_counts: Record<string, number> }[];
  }>("GET", "/task-archives");
  assert.equal(archives.total, 2);
  const northwindArchive = archives.items.find((item) => item.task_name === "Northwind external review");
  assert.ok(northwindArchive);
  assert.equal(northwindArchive.original_status, "done");
  assert.ok(northwindArchive.archived_at);
  assert.equal(northwindArchive.data_counts.findings, 3);
  assert.equal(northwindArchive.data_counts.assets, 4);

  const tasks = await mockHandle<{ tasks: { id: string }[] }>("GET", "/tasks");
  assert.equal(
    tasks.tasks.some((task) => task.id === "t-northwind"),
    true,
  );
  assert.equal(
    tasks.tasks.some((task) => task.id === "t-vpn-edge"),
    true,
  );
});

test("commands, approvals, judge usage, and notifications stay on the campaign", async () => {
  const web = await mockHandle<{ total: number; commands: { exploration_id: number; task_id: string }[] }>(
    "GET",
    "/commands?task=1",
  );
  assert.equal(web.total, 5);
  assert.ok(web.commands.every((row) => row.exploration_id === 1 && row.task_id === "t-acme-web"));
  const shop = await mockHandle<{ total: number; commands: { command: string }[] }>("GET", "/commands?task=3");
  assert.equal(shop.total, 1);
  assert.match(shop.commands[0]?.command ?? "", /order amount/);
  const other = await mockHandle<{ total: number }>("GET", "/commands?task=2");
  assert.equal(other.total, 0);

  const pending = await mockHandle<{ pending: { task_id: string; status: string; tool_input: { command: string } }[] }>(
    "GET",
    "/intercept/pending",
  );
  assert.equal(pending.pending.length, 1);
  assert.equal(pending.pending[0]?.task_id, "t-shop-pay");
  assert.match(pending.pending[0]?.tool_input.command ?? "", /order amount/);
  const webApprovals = await mockHandle<{ items: { status: string }[] }>("GET", "/intercept/task/t-acme-web");
  assert.equal(webApprovals.items.some((row) => row.status === "pending"), false);
  const shopRules = await mockHandle<{ rules: { id: number; name: string }[] }>(
    "GET",
    "/tasks/t-shop-pay/intercept-rules",
  );
  assert.equal(shopRules.rules.length, 1);
  assert.equal(shopRules.rules[0]?.name, "Payment changes require approval");

  const usage = await mockHandle<{ daily: { date: string; calls: number }[] }>("GET", "/intercept/judge/usage?days=7");
  assert.equal(usage.daily.length, 7);
  assert.equal(usage.daily.at(-1)?.date, new Date().toISOString().slice(0, 10));
  assert.ok(usage.daily.every((row) => row.calls > 0));

  const channels = await mockHandle<{ channels: { config: { url: string; headers: string } }[] }>("GET", "/notify/channels");
  assert.equal(channels.channels[0]?.config.url, "https://example.invalid/hooks/artex");
  assert.match(channels.channels[0]?.config.headers ?? "", /^__masked__/);
  const deliveries = await mockHandle<{ total: number; deliveries: { state: string; last_error: string }[] }>(
    "GET",
    "/notify/deliveries",
  );
  assert.equal(deliveries.total, 3);
  assert.equal(
    deliveries.deliveries.find((row) => row.state === "failed")?.last_error,
    "preview endpoint refused the connection",
  );
  await assert.rejects(() => mockHandle("POST", "/notify/channels/1/test"), /preview endpoint refused the connection/);

  const logs = await mockHandle<{ items: { text: string }[] }>("GET", "/logs/history");
  assert.equal(logs.items.length, 4);
  assert.ok(logs.items.some((line) => line.text.includes("t-shop-pay") && line.text.includes("f-10")));
});
