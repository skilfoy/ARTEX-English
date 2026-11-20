BEGIN;

CREATE SCHEMA artex_workspace;
REVOKE ALL ON SCHEMA artex_workspace FROM PUBLIC;

CREATE TABLE artex_workspace.workspaces (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id text NOT NULL DEFAULT current_setting('artex.workspace_user', true),
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (id, owner_id)
);

CREATE TABLE artex_workspace.records (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL,
  owner_id text NOT NULL DEFAULT current_setting('artex.workspace_user', true),
  kind text NOT NULL CHECK (kind IN ('asset', 'finding', 'evidence', 'remediation')),
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 160),
  details text NOT NULL DEFAULT '' CHECK (length(details) <= 12000),
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in progress', 'closed')),
  severity text CHECK (severity IN ('informational', 'low', 'medium', 'high', 'critical')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, owner_id) REFERENCES artex_workspace.workspaces (id, owner_id),
  CHECK (kind = 'finding' OR severity IS NULL)
);

CREATE INDEX workspaces_owner_created ON artex_workspace.workspaces (owner_id, created_at DESC, id DESC);
CREATE INDEX records_owner_workspace_updated ON artex_workspace.records (owner_id, workspace_id, updated_at DESC, id DESC);

ALTER TABLE artex_workspace.workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE artex_workspace.workspaces FORCE ROW LEVEL SECURITY;
ALTER TABLE artex_workspace.records ENABLE ROW LEVEL SECURITY;
ALTER TABLE artex_workspace.records FORCE ROW LEVEL SECURITY;

CREATE POLICY workspaces_by_owner ON artex_workspace.workspaces
  TO artex_workspace_app
  USING (owner_id = nullif(current_setting('artex.workspace_user', true), ''))
  WITH CHECK (owner_id = nullif(current_setting('artex.workspace_user', true), ''));

CREATE POLICY records_by_owner ON artex_workspace.records
  TO artex_workspace_app
  USING (owner_id = nullif(current_setting('artex.workspace_user', true), ''))
  WITH CHECK (owner_id = nullif(current_setting('artex.workspace_user', true), ''));

GRANT USAGE ON SCHEMA artex_workspace TO artex_workspace_app;
GRANT SELECT ON artex_workspace.workspaces, artex_workspace.records TO artex_workspace_app;
GRANT INSERT (name) ON artex_workspace.workspaces TO artex_workspace_app;
GRANT INSERT (workspace_id, kind, title, details, status, severity) ON artex_workspace.records TO artex_workspace_app;
GRANT UPDATE (kind, title, details, status, severity, updated_at) ON artex_workspace.records TO artex_workspace_app;

COMMIT;
