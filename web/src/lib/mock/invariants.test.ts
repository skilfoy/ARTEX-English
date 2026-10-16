import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { activity, assets, campaignLogs, commandRecords, companies, dailyTokens, findingRetestSeeds, findingTrafficSeeds, findings, llmProfiles, llmRecordDetail, llmRecords, notifyChannels, notifyDeliveries, tasks, tokenTotal, traffic, trafficBodies, workspaceList, workspaceRead, PAYMENT_HELD_COMMAND, interceptPending, interceptHistory } from "./data";

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

test("company asset counts match the asset rows", () => {
  for (const company of companies) {
    const rows = assets.filter((asset) => asset.company_id === company.id);
    assert.equal(rows.length, company.asset_count, company.name);
  }
  assert.equal(companies.find((company) => company.id === 1)?.asset_count, 26);
  assert.equal(companies.find((company) => company.id === 2)?.asset_count, 4);
});

test("session text does not quote a procedure or a credential", () => {
  const source = readFileSync(new URL("./data.ts", import.meta.url), "utf8");
  const banned = /sqlmap|impacket-|psexec|secretsdump|JdbcRowSet|JNDIExploit|hashcat|git-dumper|crackmapexec|ReverseShell|ldap:\/\/|os\.system|sudo python|sudo -l|NOPASSWD|admin\/admin123|Acme@2021|Deploy@Acme|Sql@2020|fscan|proxychains|Groovy RCE|Kerberoast|GetUserSPNs|krb5tgs/i;
  const hit = source.split("\n").find((line) => banned.test(line));
  assert.equal(hit, undefined);
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

test("workspace, commands, and transcripts stay on real tasks", () => {
  const root = workspaceList("");
  assert.ok(root.entries.some((entry) => entry.name === "t-acme-web" && entry.dir));
  const web = workspaceList("t-acme-web");
  assert.deepEqual(
    web.entries.map((entry) => entry.name).sort(),
    ["assets.md", "evidence", "scope.md"],
  );
  const scope = workspaceRead("t-acme-web/scope.md");
  assert.match(scope.content, /Assess administrative access controls/);
  assert.match(scope.content, /acme\.com/);
  const evidence = workspaceRead("t-acme-web/evidence/index.md");
  assert.match(evidence.content, /f-1: x-2/);
  assert.equal(workspaceRead("t-acme-web/missing.md").content, "");
  assert.deepEqual(
    workspaceList("t-shop-pay").entries.map((entry) => entry.name),
    ["scope.md"],
  );
  assert.equal(workspaceRead("t-northwind/scope.md").content.includes("acme.com"), false);
  const notes = [scope.content, workspaceRead("t-acme-web/assets.md").content, evidence.content].join("\n");
  assert.equal(/exploit\.py|sqlmap|wordlist/i.test(notes), false);

  assert.equal(commandRecords.filter((row) => row.task_id === "t-acme-web").length, 5);
  for (const command of commandRecords) {
    const index = tasks.findIndex((task) => task.id === command.task_id);
    assert.equal(command.exploration_id, index + 1, String(command.id));
    assert.equal(/sqlmap|nmap|ffuf|mysqldump/i.test(`${command.command} ${command.output}`), false);
  }
  assert.equal(
    commandRecords.find((row) => row.task_id === "t-shop-pay")?.command.includes(PAYMENT_HELD_COMMAND),
    true,
  );

  const session = activity.map((item) => `${item.summary ?? ""}\n${item.detail ?? ""}`).join("\n");
  const profileNames = new Set(llmProfiles.map((profile) => profile.name));
  for (const task of tasks) {
    const rows = llmRecords.filter((record) => record.task_id === task.id);
    assert.ok(rows.reduce((sum, record) => sum + record.input_tokens, 0) <= (task.tokens?.input_tokens ?? 0));
    assert.ok(rows.reduce((sum, record) => sum + record.output_tokens, 0) <= (task.tokens?.output_tokens ?? 0));
    assert.ok(rows.reduce((sum, record) => sum + record.cache_read, 0) <= (task.tokens?.cache_read_tokens ?? 0));
    assert.ok(rows.reduce((sum, record) => sum + record.cache_write, 0) <= (task.tokens?.cache_write_tokens ?? 0));
  }
  for (const record of llmRecords) {
    assert.ok(profileNames.has(record.profile_name), record.profile_name);
    const detail = llmRecordDetail(record.id);
    assert.equal(detail.id, record.id);
    const prompt = JSON.parse(detail.request_body).messages[0].content as string;
    const reply = JSON.parse(detail.response_body).content[0].text as string;
    assert.ok(session.includes(prompt), String(record.id));
    assert.ok(session.includes(reply), String(record.id));
    assert.equal(/sqlmap|curl -s/i.test(`${prompt}\n${reply}`), false);
  }
  assert.notEqual(llmRecordDetail(1).response_body, llmRecordDetail(2).response_body);
  assert.equal(llmRecordDetail(999).id, 999);
  assert.equal(llmRecordDetail(999).request_body, "");
  assert.equal(llmRecords.some((record) => record.task_id === "t-001"), false);
});

test("notifications, logs, and the one approval name real tasks", () => {
  assert.equal(interceptPending.length, 1);
  assert.equal(interceptPending[0]?.task_id, "t-shop-pay");
  assert.equal(interceptPending[0]?.tool_input.command, PAYMENT_HELD_COMMAND);
  assert.equal(interceptHistory.filter((row) => row.status === "pending").length, 1);
  assert.ok(interceptHistory.some((row) => row.task_id === "t-acme-web" && row.status !== "pending"));
  assert.equal(notifyChannels.length, 1);
  assert.equal(notifyChannels[0]?.config.url, "https://example.invalid/hooks/artex");
  assert.equal(String(notifyChannels[0]?.config.headers).startsWith("__masked__"), true);
  assert.equal(notifyDeliveries.length, 3);
  assert.equal(notifyDeliveries.find((row) => row.state === "failed")?.last_error, "preview endpoint refused the connection");
  assert.equal(notifyDeliveries.some((row) => row.severity === "critical" && row.state === "sent"), true);
  assert.equal(notifyDeliveries.some((row) => row.batch_id === 7 && row.title.includes("digest")), true);
  const findingIDs = new Set(findings.map((finding) => finding.id));
  const taskIDs = new Set(tasks.map((task) => task.id));
  for (const row of notifyDeliveries) {
    assert.ok(findingIDs.has(row.finding_id), row.finding_id);
    assert.ok([...taskIDs].some((id) => row.title.includes(id)), row.title);
  }
  assert.equal(campaignLogs.length, 4);
  for (const line of campaignLogs) {
    assert.ok([...taskIDs].some((id) => line.text.includes(id)), line.text);
    assert.ok([...findingIDs].some((id) => line.text.includes(id)), line.text);
  }
});

