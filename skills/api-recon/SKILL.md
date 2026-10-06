---
name: api-recon
description: Map frontend routes, API calls, and parameters in a locally isolated application.
---

# API reconnaissance

Map the frontend routes, API endpoints, request methods, parameters, and user interface actions of an application in a locally isolated research environment. Record observed requests and their origin. Keep static inferences separate from runtime observations.

## Scope

This workflow identifies interfaces and parameters. It does not require credential guessing, vulnerability exploitation, destructive actions, or requests to an online system. A local mock may supply login, permissions, menus, and empty business data so the frontend can render. Report the outbound requests made by the frontend, with the mock responses clearly identified.

| Layer | Evidence | Limit |
| --- | --- | --- |
| Static JavaScript | Endpoint and route candidates, possible fields | Methods and required fields need callsite or runtime confirmation. |
| Runtime | Actual method, URL, headers, body, response, and triggering action | Unvisited pages and hidden controls remain unobserved. |

## Preparation

Create an output directory such as `recon/`. Copy the relevant scripts from this skill's `scripts/` directory into it and document adjustments in `CHANGES.md`. The scripts are templates. Configure them for the application's bundle format, authentication gates, and route structure before execution. Keep generated files in the output directory.

Choose a runtime mode in `config.json`:

- `depth` uses `runtime_harvest.js` to visit routes and record requests.
- `coverage` uses the document-start `preload.js` hook while navigating visible controls.
- `both` runs depth and coverage, then merges their observations.

See [reference.md](reference.md) for configuration fields and evidence rules.

## Phase 1: Static inventory

Classify the frontend as a single page application or a multi-page application. For a single page application, copy and adjust `harvest_static.py`, then run:

```bash
python3 recon/harvest_static.py <BASE_URL> recon
wc -l recon/api_static.txt recon/routes.txt
```

The harvest collects referenced bundles and lazy chunks into `js/`, then writes `api_static.txt`, `routes.txt`, and `chunkmap.txt`. Check failed chunk downloads and correct the harvesting script before interpreting a sparse result. Search within `recon/js/` for a known endpoint's callsite and record candidate method, body, path fields, and validation rules in `param_candidates.json`.

For a multi-page application, use the adjusted `spider_mpa.py` to collect `forms.txt`, `links.txt`, and `api_inline.txt`. Exclude logout, deletion, and other state changing routes.

## Phase 2: Frontend gates

Inspect the downloaded code to identify three separate gates:

1. Login state, including the actual cookie or storage key and its decoding method.
2. Response interceptors that redirect to login or reject a business response code.
3. Permission or menu data that controls route and component visibility.

Configure `cookies`, `localStorage`, `neutralize`, and exact `stubs` based on the consumer code. A storage key inferred solely from its name is insufficient evidence. Use successful but empty mock business responses where needed. Record the source of each configured field in `CHANGES.md`.

## Phase 3: Runtime observations

Adapt `runtime_harvest.js`, `preload.js`, and `config.json`, then run the selected mode. Confirm the document-start hook loaded and the application rendered beyond its login shell.

```bash
cd recon
npm install
node runtime_harvest.js config.json
```

In coverage mode, visit each visible route, navigation item, tab, table detail, filter, sort control, and non-destructive form. Record which action triggered each request. Use multiple values for a field to distinguish fixed values, optional fields, and values derived from the interface. Preserve actual outbound headers and bodies in `scan_raw.json`, `api_detail.json`, and `param_samples.json`. A mock response does not establish a real backend response contract.

If modules remain blank, inspect the permission consumer, extract the route map with `extract_route_map.py`, build a compatible permission tree with `build_perm_tree.py`, adjust the stubs, and rerun the runtime inventory. Record routes that remain inaccessible.

## Phase 4: Consolidation

Merge static and runtime evidence into `api_merged.txt`, `params_merged.json`, and `site_map.json`. Include the frontend route, feature, method, path, transport, observed parameters, candidate fields, source, and confidence for each endpoint. Assign high confidence only when a static callsite and at least two runtime samples agree. Label untriggered fields and inaccessible modules explicitly.

The final report states the runtime mode, counts of static and observed endpoints, authentication and permission mock assumptions, unvisited modules, and all script changes. Register verified service and endpoint assets through `insert_assets` only in the locally isolated task context.
