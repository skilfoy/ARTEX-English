"use client";

// LLM Retry configured sharedware: Five layers retry their respective[Number of times + interval].
//
// Fifth floor from inside to outside: Jianlian(SDK) → Empty response(SDK) → Same provider Safety window → Polling circuit breaker → Intention to run again.
// The first three layers follow the endpoint, so each model configuration can override the global default; the last two layers are at the process level and only have a global copy..
//
// All inputs follow the same set[Leave blank = Not configured]Semantics, and backends db.RetryRule Consistent:
//   Number of times empty/0 = Use built-in default | -1 = Close this layer and try again | >0 = Use this number of times
//   interval empty/0 = Use the original index of this layer to retreat | >0 = Use this fixed millisecond interval instead

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
  /** Where this retry layer runs, and who performs it. */
  where: string;
  /** Which failures reach this layer. Do not guess from the status code alone. */
  trigger: string;
  /** Similar, but not a failure that reaches this layer. Leave it blank. Do not treat a missing response as this failure. */
  skips?: string;
  desc: string;
  attemptsLabel: string;
  /** Default value when the number of times is left blank, used as placeholder */
  defAttempts: number;
  /** Default strategy when interval is left empty, used for placeholders */
  defInterval: string;
  /** Fill in the number of times -1 meaning */
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

/** Milliseconds of human words, only used to echo next to the input box to avoid counting zeros. */
function humanMs(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${Number((ms / 1000).toFixed(2))}s`;
  return `${Number((ms / 60_000).toFixed(2))}min`;
}

/** Controlled digital input: empty string ↔ 0,Intermediate state("-","1e")Leave it locally without disturbing the parent. */
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
  // Keep up when the parent changes a complete set of values (read policies, switch configurations); you will not get here when typing by yourself,
  // Because then value Already equal to local text parse The result after.
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

/** Two knobs for one layer of retry.idPrefix Used to save when the same page appears multiple times label of htmlFor. */
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
  /** true = A compact version of the configuration drawer: leave out the expansion instructions and just keep[What error would reach this level?]This sentence */
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
        {/* Which errors will reach this level, specific to the status code——If you can't see the effect after filling in the knobs, it's probably because the error doesn't fall into this layer at all.. */}
        <p className="text-muted-foreground text-xs">
          <span className="font-medium text-foreground">Trigger</span>:{meta.trigger}
        </p>
        {!compact && meta.skips && (
          <p className="text-muted-foreground text-xs">
            <span className="font-medium text-foreground">Not retried here</span>: {meta.skips}
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
            placeholder={`Default ${meta.defAttempts}`}
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
            placeholder="Default backoff"
            onChange={(n) => onChange({ ...value, interval_ms: n })}
          />
          <span className="text-muted-foreground text-xs">{human ? `Fixed ${human}` : meta.defInterval}</span>
        </div>
      </div>
      {!compact && <p className="text-muted-foreground text-xs">Leave blank to use the default. {meta.offHint}.</p>}
    </div>
  );
}

/** Three layers of coverage in the model configuration drawer (the three layers that follow the endpoints). */
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

/** [Retry and backoff]tab:Global default value for five layers. */
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
      toast.error(`Read retry policy failed: ${(e as Error).message}`);
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
      // The backend will clamp the out-of-bounds value back into the range and return it, and refresh it directly with the returned value. What you see is what is stored..
      const saved = await api.saveLLMRetryPolicy(policy);
      setPolicy({ ...ZERO_POLICY, ...saved });
      toast.success("Saved, effective immediately (the current round of calls still uses the old parameters)");
    } catch (e) {
      toast.error(`Save failed: ${(e as Error).message}`);
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
