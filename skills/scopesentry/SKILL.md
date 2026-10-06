---
name: scopesentry-mcp
description: Query a configured ScopeSentry MCP instance for projects, assets, tasks, templates, and scan nodes.
---

# ScopeSentry MCP

Use the configured ScopeSentry MCP endpoint to inspect projects, assets, tasks, templates, and nodes in a locally isolated research environment. Follow the application's stated use conditions before creating a scan task.

## Connection

Confirm the ScopeSentry instance and its `/mcp` endpoint are reachable. Create an API key in the instance's web interface and configure the client with the returned key. Treat the key as a secret.

```json
{
  "mcpServers": {
    "scopesentry": {
      "url": "http://LOCAL_HOST:8082/mcp",
      "headers": { "X-API-Key": "YOUR_API_KEY" }
    }
  }
}
```

An instance may also accept `Authorization: Bearer YOUR_API_KEY`. Reload the MCP client and check that its tools are available. Consult each tool's schema for required fields.

## Tools

| Tool | Purpose |
| --- | --- |
| `list_projects`, `list_projects_data`, `get_project`, `create_project` | Inspect or create projects. |
| `list_tasks`, `get_task`, `create_scan_task` | Inspect or create scan tasks. |
| `list_scan_templates`, `get_scan_template`, `create_scan_template` | Inspect or create templates. |
| `list_plugin_modules`, `list_plugins` | Discover modules, plugin hashes, and default parameters. |
| `list_assets`, `count_assets`, `get_asset_detail`, `add_asset_tag` | Query and label assets. |
| `list_nodes` | Inspect available scan nodes. |

`count_assets` accepts the same `asset_type`, `search`, and `filter` fields as `list_assets` and returns a total without paging through all assets.

## Query assets

Use `list_projects` to obtain a project's object ID when the task names a project. Pass that ID in `filter.project`, where the asset type supports it. The project display name is not an object ID. Omit the filter when no project scope is supplied.

```json
{
  "asset_type": "asset",
  "pageIndex": 1,
  "pageSize": 20,
  "search": "domain=^example.com",
  "filter": { "project": ["PROJECT_OBJECT_ID"] }
}
```

The `search` field is ScopeSentry's search expression, not SQL. `==` matches an exact value, `!=` excludes a value, `&&` combines conditions, and `||` offers alternatives. `=` performs a broader pattern match. A value beginning with `^` requests a prefix match. Exact and prefix searches on indexed fields such as domain, IP, port, and title are preferable for a large asset set. The `project` restriction belongs in `filter`, not in `search`.

Common search fields across types are `tag`, `task`, and `rootDomain`. Type specific fields include:

| Asset type | Useful fields |
| --- | --- |
| `asset` | `domain`, `ip`, `port`, `service`, `app`, `title`, `statuscode`, `body`, `header` |
| `RootDomain` | `domain`, `icp`, `company` |
| `subdomain` | `domain`, `ip`, `type`, `value` |
| `UrlScan`, `DirScanResult` | `url`, `statuscode`, `redirect`, `length` where supported |
| `vulnerability` | `url`, `vulname`, `matched`, `request`, `response`, `level` |
| `IPAsset` | `ip`, `domain`, `port`, `service`, `app` |
| `SensitiveResult` | `url`, `sname`, `body`, `info`, `md5` |

Other supported asset types include `app`, `mp`, `crawler`, `PageMonitoring`, and `SubdomainTakerResult`. Filter keys depend on asset type. Common keys include `project`, `task`, `port`, `service`, `app`, `status`, `level`, `type`, and `tags`. Values under the same filter key are alternatives; different keys combine restrictions. `UrlScan` and `DirScanResult` support `sort` by `length`.

## Templates and scan tasks

Read `list_nodes` for node names and `list_scan_templates` for template object IDs. To build a template, read `list_plugin_modules` and `list_plugins` for module names, plugin hashes, and parameters, then provide those values to `create_scan_template`.

`create_scan_task` requires a task `name` and `node`; pass a template object ID in `template`. Its `targetSource` determines the target input:

| Source | Input |
| --- | --- |
| `general` | Newline separated `target` values. |
| `project` | Project object IDs in `project`. |
| `asset`, `RootDomain`, `subdomain`, `UrlScan` | `search` with optional `project`, `filter`, and `targetNumber`. |
| Sources ending in `Source` | `targetTp=search` with a query or `targetTp=select` with `targetIds`. |

For a locally isolated root domain inventory, one template can collect subdomains from the supplied roots. A later template can use `targetSource=subdomain` and `search=task=="FIRST_TASK_NAME"` to operate on the recorded subdomains. Read the first task's result with `get_task` before preparing the later task. Keep the template modules aligned with the approved local scope.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| No MCP tools | Endpoint, client configuration, and ScopeSentry service status. |
| HTTP 401 or 403 | API key and its permissions. |
| No assets | Project object ID, asset type, search fields, and filter syntax. |
| Task creation fails | Online node name, template object ID, and required target fields. |
| Slow query | Apply the known project filter, use exact or prefix searches, and reduce `pageSize`. |
