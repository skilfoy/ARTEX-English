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
