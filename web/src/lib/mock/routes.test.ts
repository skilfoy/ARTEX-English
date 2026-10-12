import assert from "node:assert/strict";
import { test } from "node:test";

process.env.MOCK_STRICT = "1";

const { MOCK_GET_ROUTES, mockHandle } = await import("./handler");

const samples: Record<string, string> = {
  ":taskId": "t-acme-web",
  ":archiveId": "1",
  ":findingId": "f-1",
  ":bindingId": "b-f-1-1",
  ":activitySeq": "1000",
  ":intentId": "i1",
  ":agentKey": "planner",
  ":conversationId": "1",
  ":messageSeq": "1",
  ":recordId": "1",
  ":mcpId": "1",
  ":skillName": "api-recon",
  ":skillFile": "SKILL.md",
  ":pendingId": "101",
  ":approvalId": "90",
};

function fill(template: string): string {
  return template.replace(/:[A-Za-z]+/g, (token) => {
    const value = samples[token];
    assert.ok(value, `missing sample id for ${token} in ${template}`);
    return value;
  });
}

test("every listed GET route resolves", async () => {
  process.env.MOCK_STRICT = "1";
  assert.ok(MOCK_GET_ROUTES.length > 0);
  for (const template of MOCK_GET_ROUTES) {
    const path = fill(template);
    const result = await mockHandle("GET", path);
    assert.notEqual(result, undefined, path);
  }
});

test("an unknown GET throws when MOCK_STRICT is set", async () => {
  process.env.MOCK_STRICT = "1";
  await assert.rejects(
    () => mockHandle("GET", "/this-route-is-not-in-the-demo"),
    /unhandled GET/,
  );
});
