import { workspaceAuth } from "@/lib/private-workspace/identity";

type AuthHandler = NonNullable<ReturnType<typeof workspaceAuth>>["handler"];
type AuthHandlers = ReturnType<AuthHandler>;

function handle(method: keyof AuthHandlers): AuthHandlers["GET"] {
  return async (request, context) => {
    const auth = workspaceAuth();
    if (!auth) return Response.json({ error: "Private workspace is unavailable." }, { status: 503 });
    return auth.handler()[method](request, context);
  };
}

export const GET = handle("GET");
export const POST = handle("POST");
