// Parsing tool for channel field tables and configuration values.
//
// Separated from the page because this one is**Data**Instead of a view: it describes what fields are there for each channel,
// What controls should be used, and form text to configuration values(JSON)bidirectional conversion of.
// After placing a separate file, you only need to move here to add a new channel, and the page itself does not need to be changed..
// The display name and introduction of the channel type. It is placed on the front end because it only affects the copywriting and the back end does not need to know.
export const KIND_LABEL: Record<string, string> = {
  dingtalk: "DingTalk",
  feishu: "Feishu",
  wecom: "Enterprise WeChat",
  webhook: "Universal Webhook",
  telegram: "Telegram",
  email: "Mail",
};

// Configuration field definitions for each channel.
//
// A front-end field table is deliberately kept here instead of being distributed by the back-end. schema:The backend is only responsible for
// Validate(Required/Format),UI What is needed is layout and control type, the two are not concerned with the same thing.
// The only coupling point is secret_keys —— Which fields should be rendered into password boxes are given by the backend,
// Because only the channel realizes that it knows which values are counted as credentials (the entire enterprise WeChat Webhook It's the credentials,
// And Dingding is only one of them secret).When adding a new channel, missing one entry here will only make the form blank.,
// Does not error silently (below hasFields will prompt).
export type FieldKind = "text" | "password" | "number" | "select" | "textarea" | "switch" | "kv" | "list";
export interface FieldDef {
  key: string;
  label: string;
  kind: FieldKind;
  placeholder?: string;
  help?: string;
  options?: { value: string; label: string }[];
}
export const CHANNEL_FIELDS: Record<string, FieldDef[]> = {
  dingtalk: [
    {
      key: "webhook",
      label: "Webhook address",
      kind: "text",
      placeholder: "https://oapi.dingtalk.com/robot/send?access_token=...",
    },
    {
      key: "secret",
      label: "Signature key",
      kind: "password",
      help: "Fill it out when \"Add Signature\" is selected for robot security settings; leave it blank if \"Custom Keywords\" is selected or security settings are not turned on.",
    },
  ],
  feishu: [
    {
      key: "webhook",
      label: "Webhook address",
      kind: "text",
      placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/...",
    },
    { key: "secret", label: "Signature verification key", kind: "password", help: "Fill in when the robot turns on \"Signature Verification\", otherwise leave it blank" },
  ],
  wecom: [
    {
      key: "webhook",
      label: "Webhook address",
      kind: "text",
      placeholder: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...",
    },
  ],
  webhook: [
    { key: "url", label: "Target URL", kind: "text", placeholder: "https://your-endpoint.example.com/hook" },
    {
      key: "method",
      label: "Request method",
      kind: "select",
      options: [
        { value: "POST", label: "POST (with request body)" },
        { value: "PUT", label: "PUT (with request body)" },
        { value: "PATCH", label: "PATCH (with request body)" },
        { value: "GET", label: "GET (without request body)" },
      ],
    },
    { key: "headers", label: "Custom request header", kind: "kv", help: "KEY=VALUE in each row, for example Authorization=Bearer xxx" },
    {
      key: "body_template",
      label: "Request body template",
      kind: "textarea",
      help:
        "Leave blank to use the built-in default template. Variables: {{.Title}} {{.Batch}} {{.Count}} {{.HomeURL}} {{.SentAt}}," +
        "and, inside range .Items: .Name, .VulnClass, .Severity, .Summary, .Assets, .DetailURL, and .StatusLabel. " +
        "Please use {{json .Xxx}} instead of {{.Xxx}} to insert a string, otherwise the quotation marks in the title will destroy the JSON.",
    },
  ],
  telegram: [
    { key: "bot_token", label: "Bot Token", kind: "password", placeholder: "123456:ABC-DEF..." },
    { key: "chat_id", label: "Chat ID", kind: "text", placeholder: "-1001234567890" },
    {
      key: "base_url",
      label: "API address",
      kind: "text",
      placeholder: "https://api.telegram.org",
      help: "Leave blank to use the official address. Set this for a self-hosted Bot API or a reverse proxy.",
    },
  ],
  email: [
    { key: "host", label: "SMTP server", kind: "text", placeholder: "smtp.example.com" },
    {
      key: "port",
      label: "Port",
      kind: "number",
      placeholder: "587",
      help: "Port 587 uses STARTTLS. For port 465, turn on \"Implicit TLS\".",
    },
    { key: "username", label: "Account", kind: "text" },
    { key: "password", label: "Password/Authorization code", kind: "password" },
    { key: "from", label: "Sender", kind: "text", placeholder: "artex@example.com" },
    { key: "to", label: "Recipient", kind: "list", help: "Multiple addresses separated by commas" },
    { key: "tls", label: "Implicit TLS", kind: "switch", help: "Turn on for port 465. Leave off for port 587 (STARTTLS starts automatically)." },
  ],
};

export const SEVERITY_OPTIONS = [
  { value: "", label: "No limit" },
  { value: "low", label: "Low risk or above" },
  { value: "medium", label: "Medium risk or above" },
  { value: "high", label: "High risk or above" },
  { value: "critical", label: "Critical only" },
];

export type ChannelForm = {
  name: string;
  kind: string;
  mode: "realtime" | "digest";
  enabled: boolean;
  ratePerMin: string;
  config: Record<string, unknown>;
  minSeverity: string;
  includeText: string;
  excludeText: string;
  taskIDsText: string;
  assetIDsText: string;
  onStatusChange: boolean;
};

export const emptyForm = (kind: string): ChannelForm => ({
  name: "",
  kind,
  mode: "realtime",
  enabled: true,
  ratePerMin: "",
  config: {},
  minSeverity: "",
  includeText: "",
  excludeText: "",
  taskIDsText: "",
  assetIDsText: "",
  onStatusChange: false,
});

// parseKV Analysis[per line KEY=VALUE]text field.
export function parseKV(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t) continue;
    const i = t.indexOf("=");
    if (i > 0) out[t.slice(0, i).trim()] = t.slice(i + 1).trim();
  }
  return out;
}
// parseIDs Parsing commas/White space separated id List.
export function parseIDs(text: string): number[] {
  return text
    .split(/[\s,,]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0);
}
// parseKeywords Parse line/Comma-separated keyword list (the vulnerability type name may contain spaces, so cut by line or comma).
export function parseKeywords(text: string): string[] {
  return text
    .split(/[\n,,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}
