import { createNeonAuth } from "@neon/auth/next/server";

export function workspaceAuth() {
  const baseUrl = process.env.NEON_AUTH_BASE_URL;
  const secret = process.env.NEON_AUTH_COOKIE_SECRET;
  return baseUrl && secret && process.env.ARTEX_WORKSPACE_DATABASE_URL
    ? createNeonAuth({ baseUrl, cookies: { secret } })
    : null;
}
