export const KINDS = {
  asset: "Assets",
  finding: "Findings",
  evidence: "Evidence",
  remediation: "Remediation",
} as const;
export const STATUSES = ["open", "in progress", "closed"] as const;
export const SEVERITIES = ["informational", "low", "medium", "high", "critical"] as const;

export type RecordKind = keyof typeof KINDS;
export type Workspace = { id: string; name: string };
export type RecordDraft = {
  kind: RecordKind;
  title: string;
  details: string;
  status: (typeof STATUSES)[number];
  severity: (typeof SEVERITIES)[number] | null;
};
export type WorkspaceRecord = RecordDraft & { id: string; updated_at: string };
export const EMPTY_DRAFT: RecordDraft = { kind: "asset", title: "", details: "", status: "open", severity: null };
