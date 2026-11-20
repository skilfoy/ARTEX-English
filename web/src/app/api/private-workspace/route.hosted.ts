import { neon } from "@neondatabase/serverless";

import { workspaceAuth } from "@/lib/private-workspace/identity";
import { KINDS, SEVERITIES, STATUSES } from "@/lib/private-workspace/model";

const uuid = /^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i;
const privateHeaders = { "Cache-Control": "private, no-store" };
const connect = (url: string) => neon(url);

function reply(body: unknown, status = 200) {
  return Response.json(body, { status, headers: privateHeaders });
}

async function authorized(run: (sql: ReturnType<typeof connect>, userId: string) => Promise<Response>) {
  try {
    const configuration = process.env.ARTEX_WORKSPACE_DATABASE_URL;
    const auth = workspaceAuth();
    if (!auth || !configuration) return reply({ error: "Private workspace is unavailable." }, 503);
    const { data } = await auth.getSession();
    if (!data?.user?.id || !data.user.emailVerified) return reply({ error: "Sign in with a verified account." }, 401);
    return await run(connect(configuration), data.user.id);
  } catch {
    return reply({ error: "The workspace could not complete this request. Try again." }, 500);
  }
}

export async function GET(request: Request) {
  const query = new URL(request.url).searchParams;
  const workspace = query.get("workspace");
  const page = Number(query.get("page") ?? 1);
  const kind = query.get("kind");
  if (workspace !== null && !uuid.test(workspace)) return reply({ error: "Invalid workspace." }, 400);
  if (!Number.isSafeInteger(page) || page < 1 || page > 100_000 || (kind !== null && !Object.hasOwn(KINDS, kind))) {
    return reply({ error: "Invalid record filter." }, 400);
  }
  return authorized(async (sql, userId) => {
    if (workspace === null) {
      const [, workspaces] = await sql.transaction([
        sql`SELECT set_config('artex.workspace_user', ${userId}, true)`,
        sql`SELECT id, name FROM artex_workspace.workspaces ORDER BY created_at DESC, id DESC`,
      ]);
      return reply({ workspaces });
    }
    const [, owned, records] = await sql.transaction([
      sql`SELECT set_config('artex.workspace_user', ${userId}, true)`,
      sql`SELECT id FROM artex_workspace.workspaces WHERE id = ${workspace}`,
      sql`SELECT id, kind, title, details, status, severity, updated_at FROM artex_workspace.records
          WHERE workspace_id = ${workspace} AND (${kind}::text IS NULL OR kind = ${kind}::text)
          ORDER BY updated_at DESC, id DESC LIMIT 26 OFFSET ${(page - 1) * 25}`,
    ]);
    return owned.length
      ? reply({ records: records.slice(0, 25), hasMore: records.length > 25 })
      : reply({ error: "Workspace not found." }, 404);
  });
}

export async function POST(request: Request) {
  if (request.headers.get("origin") !== new URL(request.url).origin)
    return reply({ error: "Invalid request origin." }, 403);
  if (!request.headers.get("content-type")?.startsWith("application/json"))
    return reply({ error: "Expected JSON." }, 415);
  const raw = await request.text();
  if (raw.length > 40000) return reply({ error: "Record is too large." }, 413);
  let body: Record<string, unknown>;
  try {
    const value = JSON.parse(raw);
    if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error();
    body = value;
  } catch {
    return reply({ error: "Invalid request." }, 400);
  }

  if (body.action === "createWorkspace") {
    const name = typeof body.name === "string" ? body.name.trim() : "";
    if (!name || name.length > 100) return reply({ error: "Enter a workspace name of up to 100 characters." }, 400);
    return authorized(async (sql, userId) => {
      const [, [workspace]] = await sql.transaction([
        sql`SELECT set_config('artex.workspace_user', ${userId}, true)`,
        sql`INSERT INTO artex_workspace.workspaces (name) VALUES (${name}) RETURNING id, name`,
      ]);
      return reply({ workspace }, 201);
    });
  }

  const { workspace, id, kind, status, severity } = body;
  const title = typeof body.title === "string" ? body.title.trim() : "";
  const details = typeof body.details === "string" ? body.details.trim() : "";
  if (
    body.action !== "saveRecord" ||
    typeof workspace !== "string" ||
    !uuid.test(workspace) ||
    (id !== undefined && (typeof id !== "string" || !uuid.test(id))) ||
    typeof kind !== "string" ||
    !Object.hasOwn(KINDS, kind) ||
    !(STATUSES as readonly unknown[]).includes(status) ||
    (severity !== null && (kind !== "finding" || !(SEVERITIES as readonly unknown[]).includes(severity))) ||
    !title ||
    title.length > 160 ||
    details.length > 12000
  )
    return reply({ error: "Check the title, record type, status, and details." }, 400);

  return authorized(async (sql, userId) => {
    const save = id
      ? sql`UPDATE artex_workspace.records SET kind = ${kind}, title = ${title}, details = ${details},
            status = ${status}, severity = ${severity}, updated_at = now()
            WHERE id = ${id} AND workspace_id = ${workspace}
            RETURNING id, kind, title, details, status, severity, updated_at`
      : sql`INSERT INTO artex_workspace.records (workspace_id, kind, title, details, status, severity)
            SELECT id, ${kind}, ${title}, ${details}, ${status}, ${severity}
            FROM artex_workspace.workspaces WHERE id = ${workspace}
            RETURNING id, kind, title, details, status, severity, updated_at`;
    const [, [record]] = await sql.transaction([sql`SELECT set_config('artex.workspace_user', ${userId}, true)`, save]);
    return record ? reply({ record }, id ? 200 : 201) : reply({ error: "Workspace or record not found." }, 404);
  });
}
