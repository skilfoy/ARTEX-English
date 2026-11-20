# Private workspace

The Vercel console opens in the interactive demo. The demo banner links to `/workspace` on deployments with private workspace access enabled. A verified account can create multiple named workspaces. Each starts empty and stores its own asset records, findings, evidence notes, and remediation records. Evidence notes can include references to files held elsewhere. Updates are stored in PostgreSQL.

## Hosting

Use a dedicated PostgreSQL database and a dedicated Neon Auth instance for this feature. Configure account creation and email verification with one-time codes. Enable email and password sign-in, require verification, send codes at sign-up, and revoke sessions on password reset. Add the Vercel production origin to the trusted origins in Neon Auth. Use a separate database and auth instance for any preview that allows sign-in. Configure a production mail sender before inviting a wider audience.

Create a login role named `artex_workspace_app` in the workspace database with a generated password. The role must have neither elevated database privileges nor permission to bypass row-level security. Run [`001_private_workspace.sql`](../db/private-workspace/001_private_workspace.sql) once as the database owner. The migration grants the application role only the schema and table privileges required for the workspace. The API obtains the account ID from the verified session and uses it for each transaction. Row-level policies and the composite foreign key keep records in their owner's workspace.

Set the following variables for the intended Vercel environment and redeploy. Use secret variables for the database URL and cookie signing secret.

| Variable | Value |
| --- | --- |
| `NEON_AUTH_BASE_URL` | The Neon Auth base URL for the workspace branch. |
| `NEON_AUTH_COOKIE_SECRET` | A random secret of at least 32 characters. |
| `ARTEX_WORKSPACE_DATABASE_URL` | The workspace database URL for the restricted `artex_workspace_app` role. |
| `NEXT_PUBLIC_PRIVATE_WORKSPACE` | Set to `1` to display the workspace entry point and sign-in screen. |

The hosted Next.js build exposes `/api/auth` and `/api/private-workspace`. The static export used by the Go installation omits these hosted API routes. The private workspace accepts manual records; its API has no task execution or scanning endpoints.
