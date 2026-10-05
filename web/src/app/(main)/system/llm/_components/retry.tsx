"use client";

// LLM 重试配置的共用件：五层重试各自的「次数 + 间隔」。
//
// 五层从内到外：建连(SDK) → 空响应(SDK) → 同 provider 安全窗口 → 轮询熔断 → 意图重跑。
// 前三层跟着端点走，所以每个模型配置都能覆盖全局默认；后两层是进程级的，只有全局一份。
//
// 所有输入都遵循同一套「留空 = 不配置」语义，与后端 db.RetryRule 一致：
//   次数   空/0 = 用内置默认 | -1 = 关闭这层重试 | >0 = 用这个次数
//   间隔   空/0 = 用这层原本的指数退避 | >0 = 改用这个固定毫秒间隔

import * as React from "react";

import { Loader2Icon, SaveIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import type { LLMRetryOverride, LLMRetryPolicy, LLMRetryRule } from "@/lib/types";

export const ZERO_RULE: LLMRetryRule = { attempts: 0, interval_ms: 0 };
export const ZERO_OVERRIDE: LLMRetryOverride = {
  connect: ZERO_RULE,
  empty: ZERO_RULE,
  stream: ZERO_RULE,
};
const ZERO_POLICY: LLMRetryPolicy = {
  ...ZERO_OVERRIDE,
  breaker: ZERO_RULE,
  intent: ZERO_RULE,
};

type LayerMeta = {
  title: string;
  /** 这层重试发生在哪、由谁执行 */
  where: string;
  /** 什么样的错误会走到这层——具体到状态码，别让人猜 */
  trigger: string;
  /** 长得像但【不】走这层的错误，省得填了没反应还以为是 bug */
  skips?: string;
  desc: string;
  attemptsLabel: string;
  /** 次数留空时的默认值，用于占位符 */
  defAttempts: number;
  /** 间隔留空时的默认策略，用于占位符 */
  defInterval: string;
  /** 次数填 -1 的含义 */
  offHint: string;
};

export const RETRY_LAYERS = {
  connect: {
    title: "Retry establishing connection",
    where: "SDK · Before getting 200",
    trigger:
      "Can't connect or haven't gotten 200 yet: connection reset/read/write timeout/DNS failure and other network layer errors, as well as HTTP 408, 429, 500, 502, 503, 504.",
    skips: "The rest of the status codes (400 / 401 / 403 / 404 / 413 / 422, etc.) are deterministic rejections. Retransmission will also fail and will be thrown directly.",
    desc: "Resend the same request unchanged. Once the flow starts (200 has been obtained), if it is disconnected in the middle, it will not belong to this layer of management.",
    attemptsLabel: "Number of retries",
    defAttempts: 3,
    defInterval: "0.5s→1s→2s index (capped at 8s)",
    offHint: "-1 = Do not retry once, throw up immediately if failed",
  },
  empty: {
    title: "Retry with empty response",
    where: "SDK · openai format only",
    trigger:
      "HTTP 200, finish_reason is a normal stop, but the entire response does not have a content block - the gateway empty frame, the thought field frame loss, and the sampling hiccup will all look like this.",
    skips: "Things that have no content due to max_tokens truncation are not counted (this needs to be solved by raising the output upper limit, and resending will only cause another collision).",
    desc: "Resend the entire prompt, so it is more expensive in long contexts, and the number of times should not be large.",
    attemptsLabel: "Number of retries",
    defAttempts: 2,
    defInterval: "0.5s→1s→2s index (capped at 8s)",
    offHint: "-1 = Empty response is handed over directly as is",
  },
  stream: {
    title: "Retry with provider security window",
    where: "This project · Before delivery and output",
    trigger:
      "The problem occurred after the flow was established (got 200): the connection was disconnected midway, the provider was overloaded, 429 / 5xx error events in the flow - and a token was not handed over to the caller.",
    skips:
      "Quota exhausted (402 / insufficient_quota, handed over to polling for configuration), context too long (413 / context length, handed over to compression), 400 / 401 / 403 / 404 / 422 deterministic rejection, no retry.",
    desc: "Replay the same request on the same configuration. Replay does not repeat model output or tool execution because no output has been delivered yet.",
    attemptsLabel: "Number of retries",
    defAttempts: 2,
    defInterval: "0.5s→1s index (capped at 4s)",
    offHint: "-1 = Cut off the flow and hand it over directly to the outer layer for re-run.",
  },
  breaker: {
    title: "Polling circuit breaker",
    where: "This project · Process level, global copy",
    trigger:
      "Instantaneous failures (429, 5xx, network errors) will be blown when they continuously accumulate to the threshold; deterministic failures such as insufficient balance (402), key invalidation (401/403), and model non-existence (404) do not look at the threshold and will be blown at the first time.",
    skips: "It will be cleared once it is successful, so the configuration with occasional convulsions will not be slowly accumulated until it fuses.",
    desc: "Enter cooling after the fuse is blown. During the cooling period, polling will directly skip this configuration. The status is dropped into the library and will not be lost after restarting.",
    attemptsLabel: "Failed to blow out several times in a row",
    defAttempts: 3,
    defInterval: "1min→5min→30min gradient",
    offHint: "-1 = Never blow out on instantaneous failure (still blow out on deterministic failure)",
  },
  intent: {
    title: "Intention to run again",
    where: "This project · Process level, global copy",
    trigger:
      "The first few layers are not caught: the worker ends with model_error - all inner layer retries are exhausted, or the stream is interrupted after it has started delivering output (replay is not safe at that time, and the entire stream can only be restarted).",
    skips: "The quota exhaustion has been processed by polling and configuration change, and will not be re-run here; the task will give way immediately when it is suspended/terminated/enters the end, and does not take up the back-off time.",
    desc: "Run the entire intention from the beginning again. It is the outermost layer. A rerun means that the times of the inner layers will be multiplied again.",
    attemptsLabel: "Number of reruns",
    defAttempts: 2,
    defInterval: "Fixed 3s",
    offHint: "-1 = No rerun, the intention is directly judged as blocked",
  },
} satisfies Record<string, LayerMeta>;

type LayerKey = keyof typeof RETRY_LAYERS;

/** 毫秒的人话，只用于在输入框旁边回显，免得数零。 */
function humanMs(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${Number((ms / 1000).toFixed(2))}s`;
  return `${Number((ms / 60_000).toFixed(2))}min`;
}

/** 受控数字输入：空串 ↔ 0，中间态（"-"、"1e"）原样留在本地，不打扰父级。 */
function NumField({
  id,
  value,
  onChange,
  placeholder,
  min,
}: {
  id: string;
  value: number;
  onChange: (n: number) => void;
  placeholder: string;
  min: number;
}) {
  const [text, setText] = React.useState(value === 0 ? "" : String(value));
  // 父级换了一整套值（读取到策略、切换配置）时跟上；自己敲字时不会走到这里，
  // 因为那时 value 已经等于本地文本 parse 后的结果。
  React.useEffect(() => {
    const incoming = value === 0 ? "" : String(value);
    setText((cur) => (Number(cur || 0) === value ? cur : incoming));
  }, [value]);
  return (
    <Input
      id={id}
      type="number"
      min={min}
      className="w-28 shrink-0"
      value={text}
      placeholder={placeholder}
      onChange={(e) => {
        setText(e.target.value);
        const n = Number(e.target.value);
        onChange(e.target.value.trim() === "" || !Number.isFinite(n) ? 0 : Math.trunc(n));
      }}
    />
  );
}

/** 一层重试的两个旋钮。idPrefix 用来在同一页出现多次时保住 label 的 htmlFor。 */
export function RetryRuleFields({
  layer,
  idPrefix,
  value,
  onChange,
  compact,
}: {
  layer: LayerKey;
  idPrefix: string;
  value: LLMRetryRule;
  onChange: (r: LLMRetryRule) => void;
  /** true = 配置抽屉里的紧凑版：省掉展开说明，只留「什么错误会走到这层」这一句 */
  compact?: boolean;
}) {
  const meta = RETRY_LAYERS[layer];
  const human = humanMs(value.interval_ms);
  return (
    <div className={compact ? "grid gap-2" : "grid gap-3 rounded-lg border p-3"}>
      <div className="grid gap-0.5">
        <div className="flex flex-wrap items-baseline gap-2">
          <Label className="text-sm">{meta.title}</Label>
          <span className="text-muted-foreground text-xs">{meta.where}</span>
        </div>
        {/* 哪些错误会走到这层，具体到状态码——填了旋钮却看不到效果，多半是错误压根不落在这层。 */}
        <p className="text-muted-foreground text-xs">
          <span className="font-medium text-foreground">Trigger</span>：{meta.trigger}
        </p>
        {!compact && meta.skips && (
          <p className="text-muted-foreground text-xs">
            <span className="font-medium text-foreground">Don't take this floor</span>：{meta.skips}
          </p>
        )}
        {!compact && <p className="text-muted-foreground text-xs">{meta.desc}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex items-center gap-2">
          <Label htmlFor={`${idPrefix}-${layer}-n`} className="text-muted-foreground text-xs">
            {meta.attemptsLabel}
          </Label>
          <NumField
            id={`${idPrefix}-${layer}-n`}
            min={-1}
            value={value.attempts}
            placeholder={`Default${meta.defAttempts}`}
            onChange={(n) => onChange({ ...value, attempts: n })}
          />
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor={`${idPrefix}-${layer}-ms`} className="text-muted-foreground text-xs">
            Interval ms
          </Label>
          <NumField
            id={`${idPrefix}-${layer}-ms`}
            min={0}
            value={value.interval_ms}
            placeholder="Default retreat"
            onChange={(n) => onChange({ ...value, interval_ms: n })}
          />
          <span className="text-muted-foreground text-xs">{human ? `Fixed${human}` : meta.defInterval}</span>
        </div>
      </div>
      {!compact && <p className="text-muted-foreground text-xs">Leave blank = use default;{meta.offHint}。</p>}
    </div>
  );
}

/** 模型配置抽屉里的三层覆盖（跟着端点走的那三层）。 */
export function ProfileRetryFields({
  value,
  onChange,
}: {
  value: LLMRetryOverride;
  onChange: (o: LLMRetryOverride) => void;
}) {
  return (
    <div className="grid gap-3 rounded-lg border p-3">
      <div className="grid gap-0.5">
        <Label className="text-sm">Retry coverage</Label>
        <p className="text-muted-foreground text-xs">
          Only takes effect for this configuration, overriding the global default in "Retry and Backoff". Leave each box blank = follow the overall situation; fill in -1 for the number of times = turn off this layer and try again;
          Once the interval is filled, the exponential backoff is replaced by a fixed interval. Circuit breakers and intent reruns are at the process level and can only be adjusted on the global page.
        </p>
      </div>
      {(["connect", "empty", "stream"] as const).map((k) => (
        <div key={k} className="border-t pt-3 first:border-t-0 first:pt-0">
          <RetryRuleFields
            compact
            layer={k}
            idPrefix="pf"
            value={value[k]}
            onChange={(r) => onChange({ ...value, [k]: r })}
          />
        </div>
      ))}
    </div>
  );
}

/** 「重试与退避」tab：五层的全局默认值。 */
export function RetryPolicyPanel() {
  const [policy, setPolicy] = React.useState<LLMRetryPolicy>(ZERO_POLICY);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const p = await api.llmRetryPolicy();
      setPolicy({ ...ZERO_POLICY, ...p });
    } catch (e) {
      toast.error(`Read retry policy failed:${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  async function save() {
    if (saving) return;
    setSaving(true);
    try {
      // 后端会把越界值夹回区间并回传，直接用回传值刷新，所见即所存。
      const saved = await api.saveLLMRetryPolicy(policy);
      setPolicy({ ...ZERO_POLICY, ...saved });
      toast.success("Saved, effective immediately (the current round of calls still uses the old parameters)");
    } catch (e) {
      toast.error(`Save failed:${(e as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  const set = (k: LayerKey) => (r: LLMRetryRule) => setPolicy((p) => ({ ...p, [k]: r }));

  if (loading) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-dashed p-10 text-muted-foreground text-sm">
        <Loader2Icon className="size-4 animate-spin" /> Read retry strategy…
      </div>
    );
  }

  return (
    <div className="grid gap-4">
      <div className="rounded-lg border bg-muted/30 p-3 text-muted-foreground text-xs leading-relaxed">
        The failure of a model call will go through five layers of retries in sequence, from inside to outside:
        <span className="text-foreground"> Establish connection → Empty response → Same as provider security window → Polling circuit breaker → Intention to rerun</span>
        . When the inner layer is exhausted, it is the turn of the outer layer, so the number of times is
        <span className="text-foreground">Multiply</span>
          If each layer is filled up, dozens of requests can be burned in one jitter.
        Leaving everything blank is the current default value, which is exactly the same as the behavior without this page. The first three layers can be overridden individually in each model configuration.
      </div>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {(Object.keys(RETRY_LAYERS) as LayerKey[]).map((k) => (
          <RetryRuleFields key={k} layer={k} idPrefix="gl" value={policy[k]} onChange={set(k)} />
        ))}
      </div>

      <div className="flex gap-2">
        <Button onClick={save} disabled={saving}>
          {saving ? <Loader2Icon className="animate-spin" /> : <SaveIcon />}
          Save
        </Button>
        <Button variant="outline" onClick={() => setPolicy(ZERO_POLICY)} disabled={saving}>
          Restore all to default
        </Button>
      </div>
    </div>
  );
}
