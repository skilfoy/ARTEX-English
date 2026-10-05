import type { NewAssetType } from "@/lib/types";

const ASSET_TYPE_LABELS: Record<NewAssetType, string> = {
  app: "Application",
  endpoint: "Interface",
  ip: "IP",
  root_domain: "Root domain name",
  service: "Service",
  subdomain: "Subdomain name",
};

const TASK_ASSET_SOURCE_LABELS: Record<string, string> = {
  agent: "Agent found",
  anchor: "Blackboard Anchor",
  api: "Asset API",
  company: "Corporate relations",
  legacy: "Historical connection",
  manual: "Manual addition",
  system: "System association",
  task: "Task initialization",
};

export function taskAssetTypeLabel(type: NewAssetType): string {
  return ASSET_TYPE_LABELS[type];
}

export function taskAssetSourceLabel(source: string): string {
  return TASK_ASSET_SOURCE_LABELS[source] ?? source;
}
