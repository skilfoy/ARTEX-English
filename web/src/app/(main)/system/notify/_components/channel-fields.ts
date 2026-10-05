// 渠道字段表与配置值的解析工具。
//
// 与页面拆开是因为这一份是**数据**而不是视图：它描述每种渠道有哪些字段、
// 各自该用什么控件，以及表单文本到配置值（JSON）的双向转换。
// 单独放一个文件后，新增渠道只需要动这里，页面本身不必改。
// 渠道类型的展示名与简介。放在前端是因为它只影响文案，后端不需要知道。
export const KIND_LABEL: Record<string, string> = {
  dingtalk: "DingTalk",
  feishu: "Feishu",
  wecom: "Enterprise WeChat",
  webhook: "Universal Webhook",
  telegram: "Telegram",
  email: "Mail",
};

// 各渠道的配置字段定义。
//
// 这里刻意保留一份前端字段表，而不是让后端下发 schema：后端只负责
// Validate（必填/格式），UI 需要的是布局与控件类型，两者关注的不是同一件事。
// 唯一的耦合点是 secret_keys —— 哪些字段该渲染成密码框由后端给出，
// 因为只有渠道实现自己清楚哪些值算凭据（企业微信的整个 Webhook 就是凭据，
// 而钉钉的只是其中一个 secret）。新增渠道时这里少一个条目只会让表单变空白，
// 不会静默出错（下面的 hasFields 会提示）。
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
        "And .Name/.VulnClass/.Severity/.Summary/.Assets/.DetailURL/.StatusLabel under range .Items." +
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
      help: "Leave it blank and use the official address; fill it in when you create a self-built Bot API and reverse it.",
    },
  ],
  email: [
    { key: "host", label: "SMTP server", kind: "text", placeholder: "smtp.example.com" },
    {
      key: "port",
      label: "Port",
      kind: "number",
      placeholder: "587",
      help: "587 Go to STARTTLS; 465 Please turn on \"Implicit TLS\"",
    },
    { key: "username", label: "Account", kind: "text" },
    { key: "password", label: "Password/Authorization code", kind: "password" },
    { key: "from", label: "Sender", kind: "text", placeholder: "artex@example.com" },
    { key: "to", label: "Recipient", kind: "list", help: "Multiple addresses separated by commas" },
    { key: "tls", label: "Implicit TLS", kind: "switch", help: "465 port is open; 587 remains closed (will automatically STARTTLS)" },
  ],
};

export const SEVERITY_OPTIONS = [
  { value: "", label: "No limit" },
  { value: "low", label: "Low risk or above" },
  { value: "medium", label: "Moderately dangerous or above" },
  { value: "high", label: "High risk or above" },
  { value: "critical", label: "Severe only" },
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

// parseKV 解析「每行 KEY=VALUE」的文本域。
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
// parseIDs 解析逗号/空白分隔的 id 列表。
export function parseIDs(text: string): number[] {
  return text
    .split(/[\s,，]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0);
}
// parseKeywords 解析行/逗号分隔的关键词列表（漏洞类型名可能含空格，所以按行或逗号切）。
export function parseKeywords(text: string): string[] {
  return text
    .split(/[\n,，]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}
