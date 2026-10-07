# API reconnaissance reference

This reference lists the configuration fields and evidence checks used by [the workflow](SKILL.md). Copy scripts into the output directory and record changes in `CHANGES.md`.

## Runtime configuration

```json
{
  "baseUrl": "http://local-target/",
  "runtimeMode": "both",
  "chromium": "/usr/bin/chromium",
  "cookies": [],
  "localStorage": {},
  "neutralize": {
    "fields": ["response_code", "code", "errno", "status"],
    "success": 0,
    "flags": { "success": true, "message": "ok" }
  },
  "forward": false,
  "loginUrlPattern": "/login",
  "apiPattern": "/api/|/rest/|/graphql",
  "mockTier": "L1+L2",
  "recordDetail": true,
  "observe": { "storageReads": false, "cookieReads": false, "xhrHeaders": true },
  "neutralizeVueRouter": false,
  "stubs": [],
  "explore": { "clickTabs": true, "clickTables": true, "pushStateFallback": true, "maxMenuItems": 50 },
  "routes": [],
  "waitMs": 1500,
  "perRouteMs": 900,
  "headless": true,
  "waitUntil": "domcontentloaded",
  "routeTimeout": 12000,
  "captureResponses": true,
  "recordWs": true,
  "respMax": 600
}
```

Confirm each cookie, storage key, response code, and permission shape in the downloaded frontend code. `cookies[].value` accepts a plain value, `json:` followed by JSON, or `b64json:` followed by JSON that the script encodes. `forward` allows the runtime script to pass requests to the configured backend. Keep it false for an isolated mock. `runtimeMode` accepts `depth`, `coverage`, or `both`.

An exact stub can supply a locally isolated permission response:

```json
{
  "match": "permissions/all",
  "body": {
    "response_code": 0,
    "data": [{ "code": "DASHBOARD", "position": 1, "children": [] }]
  }
}
```

Match the field names and nesting expected by the frontend consumer. A flat list of granted codes and a menu tree may require separate stubs. `neutralize.fields` and `neutralize.success` must match the response interceptor's actual behavior.

## Locating frontend gates

Search only the downloaded bundles in `recon/js/`. Bound output so a large minified bundle does not flood the working context.

```bash
rg -o 'isLogin|isAuthenticated|getToken|localStorage|sessionStorage|document.cookie' recon/js | head -30
rg -o 'interceptors.response|response_code|errcode|errno|router.push|router.replace' recon/js | head -30
rg -o 'permission|role_permissions|menuList|routeMap|userRouteAuth|hasPermission' recon/js | head -30
```

Trace the login predicate back to its storage read and decoder. Trace the response interceptor to its success and login failure branches. Trace menu and permission responses to the code that renders routes and buttons. A route name or storage key alone does not establish a working mock.

## Parameter evidence

For each known endpoint, inspect its callsite, request wrapper, form validation, and transport. Search for `params`, `data`, `body`, `FormData`, GraphQL variables, and path substitutions near the callsite. Record fields from runtime request bodies and query strings across multiple actions and values.

| Confidence | Evidence |
| --- | --- |
| High | Static callsite and at least two runtime samples agree. |
| Medium | Static callsite or one runtime sample. |
| Low | Inferred from response or validation error without a matching request. |
| Untriggered | Candidate field exists in code, but its control was not reached. |

A missing required field may be recorded from a local validation response; identify that response as the evidence. Inspect nested `data`, `bizData`, or GraphQL `variables` before naming top-level parameters. If encryption transforms a request, capture the pre-encryption values at the local function boundary and state that the wire representation differs.

## Permission tree recovery

Use `extract_route_map.py` and `build_perm_tree.py` when a rendered shell has an empty menu or modules remain hidden after authentication is mocked. Inspect the source's route map and permission tree constructor, adjust the scripts, and compare generated routes with `routes.txt`. Rerun the runtime inventory after updating stubs. List inaccessible routes in the report.

## Output contract

`site_map.json` combines the site identifier, runtime mode, application type, mock assumptions, static endpoints, observed endpoints, routes, features, parameters, and limitations. `api_merged.txt` uses one method and path per line with its evidence source. `param_samples.json` retains the action that triggered each request. Preserve `CHANGES.md` so a reader can reproduce the local inventory.
