import assert from "node:assert/strict";
import test from "node:test";
import { mockHandle } from "./handler";

function hostOf(asset: { domain?: string; url?: string; ip?: string; root_domain?: string }): string {
  return [asset.domain, asset.root_domain, asset.url, asset.ip].filter(Boolean).join(" ");
}

test("a company selection hides the other organization's tasks, hosts, and findings", async () => {
  const allTasks = await mockHandle<{ tasks: { id: string }[] }>("GET", "/tasks");
  assert.ok(allTasks.tasks.some((task) => task.id === "t-acme-web"));
  assert.ok(allTasks.tasks.some((task) => task.id === "t-northwind"));

  const northwindTasks = await mockHandle<{ tasks: { id: string }[] }>("GET", "/tasks?company_id=2");
  assert.deepEqual(
    northwindTasks.tasks.map((task) => task.id),
    ["t-northwind"],
  );
  const acmeTasks = await mockHandle<{ tasks: { id: string }[] }>("GET", "/tasks?company_id=1");
  assert.ok(acmeTasks.tasks.some((task) => task.id === "t-acme-web"));
  assert.equal(
    acmeTasks.tasks.some((task) => task.id === "t-northwind"),
    false,
  );

  const northwindAssets = await mockHandle<{ assets: { domain?: string; url?: string; ip?: string }[] }>(
    "GET",
    "/assets?company_id=2&limit=100",
  );
  assert.ok(northwindAssets.assets.length > 0);
  assert.ok(northwindAssets.assets.every((asset) => !hostOf(asset).includes("acme.com")));
  assert.ok(northwindAssets.assets.some((asset) => hostOf(asset).includes("northwind.example")));
  const acmeAssets = await mockHandle<{ assets: { domain?: string; url?: string; ip?: string }[] }>(
    "GET",
    "/assets?company_id=1&limit=100",
  );
  assert.ok(acmeAssets.assets.length > 0);
  assert.ok(acmeAssets.assets.every((asset) => !hostOf(asset).includes("northwind.example")));
  assert.ok(acmeAssets.assets.some((asset) => hostOf(asset).includes("acme.com")));

  const northwindFindings = await mockHandle<{ items: { id: string; task_id?: string }[] }>(
    "GET",
    "/exploration/findings?company_id=2&page=1&limit=100",
  );
  assert.ok(northwindFindings.items.length > 0);
  assert.ok(northwindFindings.items.every((finding) => finding.task_id === "t-northwind"));
  assert.equal(
    northwindFindings.items.some((finding) => finding.id === "f-1" || hostOf({ domain: finding.task_id }).includes("acme")),
    false,
  );
  const acmeFindings = await mockHandle<{ items: { id: string; task_id?: string }[] }>(
    "GET",
    "/exploration/findings?company_id=1&page=1&limit=100",
  );
  assert.ok(acmeFindings.items.some((finding) => finding.id === "f-1"));
  assert.equal(
    acmeFindings.items.some((finding) => finding.task_id === "t-northwind"),
    false,
  );
});
