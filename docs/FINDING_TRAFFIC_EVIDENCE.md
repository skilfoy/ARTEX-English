# Traffic evidence for findings

The finding detail view supports selecting traffic across pages, assigning a role and note to each record, ordering the records, and removing bindings. The traffic view can bind several records to an existing finding. Evidence inherited from another task is read-only in the receiving task.

## Automatic binding

The `agent_traffic_binding` setting is available through `/api/settings` and is disabled by default. Enabling it gives the reporting agent access to traffic search and retrieval tools. That agent verifies traffic records, binds them to a finding, reads the resulting evidence version, and saves a report against that version. This process consumes additional model tokens for tool calls and traffic review. A new agent round reads the current setting.

Disabling automatic binding removes the traffic-reference parameters, the bind-finding tool, and the automatic-binding prompt guidance from subsequent agent rounds. The reporting agent does not receive traffic search or retrieval tools while the setting is off. Running sessions cannot submit new automatic bindings after the setting is disabled. Manual binding, traffic capture, saved evidence, and export remain available.

`report_finding` also accepts explicit `traffic_refs` or an `evidence_hint_id`. Each referenced traffic record must exist and have a complete body. Binding and finding creation use a transaction, so an invalid reference rejects the submission. Repeatedly adding the same snapshot leaves the existing binding and note intact. Traffic evidence is optional for findings that lack a recorded HTTP request, including findings concerning other protocols. Agents can retain command output and logs as evidence.

## Agent tools and identifiers

`traffic_refs` is an ordered array. Each entry identifies an existing traffic record and can specify its role and a note:

```json
{
  "traffic_refs": [
    {"traffic_id": "record-123", "role": "baseline", "note": "Control request"},
    {"traffic_id": "record-456", "role": "proof", "note": "Verification request"}
  ]
}
```

The available roles are `baseline`, `proof`, `verification`, and `supporting`. The default is `supporting`. Agents should verify candidate records with `traffic_search` and `traffic_get`. A matching hostname or timestamp alone does not establish that a record belongs to a task.

`report_finding` returns `finding_id` for the independent finding record and `finding_node_id` for its exploration graph node. `get_finding_traffic(finding_id)` uses the finding record ID and returns ordered bindings with their `version`. `update_finding_report` continues to use the exploration node ID. Its `evidence_version` parameter identifies the version reviewed for the report. An evidence change during report generation rejects a write based on the previous version.

An agent can use `bind_finding_traffic(finding_id, traffic_refs)` to add evidence to an existing finding. `add_hint` and `add_task_hint` can carry `traffic_refs`; the planner can select references from a hint in the same task with `evidence_hint_id`. The system does not infer bindings from domains, timestamps, or browsing history.

## HTTP API

The base path is `/api/exploration/findings/{finding_id}/traffic`. Authentication and task visibility rules apply. Inherited findings are read-only.

| Method and path | Operation |
| --- | --- |
| `GET /` | List bindings and evidence versions |
| `POST /` | Append an ordered `traffic_refs` array |
| `PATCH /{binding_id}` | Update `version`, `role`, or `note` |
| `DELETE /{binding_id}` | Remove a binding using its current `version` |
| `PUT /order` | Submit a complete `binding_ids` list with its current `version` |
| `GET /{binding_id}` | Read snapshot metadata and a bounded body preview |
| `GET /{binding_id}/body` | Read `request` or `response` bytes by `offset` and `length`, or use `download=1` |

Body reads are limited to 8,192 bytes per segment. Version and ordering conflicts, or writes during archival, return `409`. Writes to inherited evidence return `403`. A missing binding or a binding outside the finding returns `404`.

## Storage, backup, and export

Startup migrations create `traffic_evidence_snapshots` and `finding_traffic_bindings` and add `evidence_version` and `report_evidence_version` to findings. The version columns default to zero.

A snapshot records the original traffic ID, capture time, URL, method, status, headers, body lengths, and SHA-256 hashes. Request and response bodies are stored by hash under `<data>/evidence/blobs/<first-two-hash-characters>/<hash>.bin`. The evidence directory is separate from the traffic capture directory. Several findings can share a snapshot or body. Reads and exports verify stored content.

The binding operation reads a complete traffic record under its lock, persists and verifies body files, and writes graph, finding, snapshot, and binding records in one PostgreSQL transaction. The planner receives a notification after commit. PostgreSQL advisory lock `7337741004` coordinates evidence files with database references. The cleanup process runs hourly and waits at least 24 hours before removing unreferenced content. Backups should include both PostgreSQL and `data/evidence`.

Markdown exports include the ordered evidence list and version. JSON exports include metadata, and CSV exports include binding counts and IDs. The `md-zip` export includes the finding Markdown and the following files for each binding:

```text
evidence/<finding_id>/<binding_id>/
  manifest.json
  request.http
  response.http
  request.bin
  response.bin
```

Downloads verify attachments and archive integrity before delivery. Archive version 3 packages evidence snapshots and bodies independently of the original traffic records. Restore verifies bodies before committing metadata. Archive versions 1 and 2 remain supported.

## Validation

Use a separate PostgreSQL test database for each package and set `ARTEX_PG_DSN` explicitly. The relevant Go checks are:

```bash
go test ./evidence ./db ./agent ./server -count=1
go test -race -p 1 ./evidence ./db ./agent ./server -run 'TestEvidence|TestFindingTraffic|TestFindingEvidence|TestReportFindingAtomicContract|TestTaskArchive'
```

Frontend checks include TypeScript validation, Biome checks for changed files, the normal build, and a static export. The evidence feature uses a global coordination lock, so large exports or slow attachment transfers can delay other evidence operations.
