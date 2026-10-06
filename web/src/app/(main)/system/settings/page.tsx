"use client";

import * as React from "react";

import { CpuIcon, FlaskConicalIcon, KeyboardIcon, RadioTowerIcon, SearchIcon, ShieldAlertIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import { CHAT_SEND_MODE_OPTIONS, type ChatSendMode, setChatSendMode, useChatSendMode } from "@/lib/chat-send-mode";
import type { Settings } from "@/lib/types";

import { UpdateCard } from "./_components/update-card";

export default function SystemSettingsPage() {
  const [trafficCapture, setTrafficCapture] = React.useState(false);
  const [agentTrafficBinding, setAgentTrafficBinding] = React.useState(false);
  const [webSearch, setWebSearch] = React.useState(false);
  const [backend, setBackend] = React.useState("ddgs");
  const [braveKeySet, setBraveKeySet] = React.useState(false);
  const [braveKeyInput, setBraveKeyInput] = React.useState("");
  const [tavilyKeySet, setTavilyKeySet] = React.useState(false);
  const [tavilyKeyInput, setTavilyKeyInput] = React.useState("");
  const [savingTavilyKey, setSavingTavilyKey] = React.useState(false);
  const [proxyInput, setProxyInput] = React.useState("");
  const [savingProxy, setSavingProxy] = React.useState(false);
  const [globalProxyInput, setGlobalProxyInput] = React.useState("");
  const [savingGlobalProxy, setSavingGlobalProxy] = React.useState(false);
  const [testing, setTesting] = React.useState(false);
  const [loaded, setLoaded] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [savingKey, setSavingKey] = React.useState(false);
  const [pyInterp, setPyInterp] = React.useState("");
  const [workers, setWorkers] = React.useState("3");
  const [savingWorkers, setSavingWorkers] = React.useState(false);
  // Operation constraint injection range(Open by default).
  const [injectPlanner, setInjectPlanner] = React.useState(true);
  const [injectWorker, setInjectWorker] = React.useState(true);
  // Experimental features:noa Context compression(Default off).
  const [noaCompaction, setNoaCompaction] = React.useState(false);
  // Pure front-end preference: don't go /api/settings,Direct reading and writing localStorage.
  const sendMode = useChatSendMode();

  const apply = React.useCallback((s: Settings) => {
    setTrafficCapture(!!s.traffic_capture);
    setAgentTrafficBinding(!!s.agent_traffic_binding);
    setWebSearch(!!s.web_search_enabled);
    setBackend(s.web_search_backend || "ddgs");
    setBraveKeySet(!!s.brave_key_set);
    setTavilyKeySet(!!s.tavily_key_set);
    setProxyInput(s.web_search_proxy ?? "");
    setGlobalProxyInput(s.global_proxy ?? "");
    setPyInterp(s.python_interpreter ?? "");
    setWorkers(String(s.workers ?? 3));
    setInjectPlanner(s.constraints_inject_planner !== false);
    setInjectWorker(s.constraints_inject_worker !== false);
    setNoaCompaction(!!s.noa_compaction);
  }, []);

  const saveWorkers = () => {
    const n = Number(workers);
    if (!Number.isInteger(n) || n <= 0) {
      toast.error("The concurrency number must be an integer greater than 0");
      return;
    }
    setSavingWorkers(true);
    api
      .setSettings({ workers: n })
      .then((s) => {
        apply(s);
        toast.success("The number of saved concurrent work agents (effective for tasks started later)");
      })
      .catch((e) => toast.error("Save failed:" + (e as Error).message))
      .finally(() => setSavingWorkers(false));
  };

  const savePython = () => {
    setSaving(true);
    api
      .setSettings({ python_interpreter: pyInterp.trim() })
      .then((s) => {
        apply(s);
        toast.success("Saved Python interpreter configuration");
      })
      .catch((e) => toast.error("Save failed:" + (e as Error).message))
      .finally(() => setSaving(false));
  };
  const detectPython = () => {
    setSaving(true);
    api
      .detectPython()
      .then((r) => setPyInterp(r.python_interpreter))
      .catch(() => undefined)
      .finally(() => setSaving(false));
  };

  React.useEffect(() => {
    api
      .settings()
      .then(apply)
      .catch(() => undefined)
      .finally(() => setLoaded(true));
  }, [apply]);

  const toggleTraffic = (v: boolean) => {
    setTrafficCapture(v); // optimistic
    setSaving(true);
    api
      .setSettings({ traffic_capture: v })
      .then(apply)
      .catch(() => setTrafficCapture(!v)) // revert on failure
      .finally(() => setSaving(false));
  };

  const toggleInjectPlanner = (v: boolean) => {
    setInjectPlanner(v); // optimistic
    api
      .setSettings({ constraints_inject_planner: v })
      .then(apply)
      .catch(() => setInjectPlanner(!v)); // revert on failure
  };

  const toggleAgentTrafficBinding = (v: boolean) => {
    setAgentTrafficBinding(v);
    setSaving(true);
    api
      .setSettings({ agent_traffic_binding: v })
      .then((s) => {
        apply(s);
        toast.success(v ? "Agent automatic binding traffic is turned on" : "Agent automatic binding traffic is turned off");
      })
      .catch((e) => {
        setAgentTrafficBinding(!v);
        toast.error(`Save failed:${(e as Error).message}`);
      })
      .finally(() => setSaving(false));
  };

  const toggleInjectWorker = (v: boolean) => {
    setInjectWorker(v); // optimistic
    api
      .setSettings({ constraints_inject_worker: v })
      .then(apply)
      .catch(() => setInjectWorker(!v)); // revert on failure
  };

  const toggleNoaCompaction = (v: boolean) => {
    setNoaCompaction(v); // optimistic
    api
      .setSettings({ noa_compaction: v })
      .then((s) => {
        apply(s);
        toast.success(v ? "noa context compression is enabled (effective for subsequent runs)" : "noa context compression turned off (restores built-in compression)");
      })
      .catch((e) => {
        setNoaCompaction(!v); // revert on failure
        toast.error(`Save failed:${(e as Error).message}`);
      });
  };

  // Persist a web-search patch (enable and/or backend). Optimistic with refetch.
  const saveWebSearch = (patch: Partial<Settings>) => {
    setSaving(true);
    api
      .setSettings(patch)
      .then((s) => {
        apply(s);
        toast.success("Saved web search configuration");
      })
      .catch((e) => {
        toast.error("Save failed:" + (e as Error).message);
        api
          .settings()
          .then(apply)
          .catch(() => undefined);
      })
      .finally(() => setSaving(false));
  };

  const saveBraveKey = () => {
    setSavingKey(true);
    api
      .setSettings({ brave_search_api_key: braveKeyInput })
      .then((s) => {
        apply(s);
        setBraveKeyInput("");
        toast.success("Saved Brave API Key");
      })
      .catch((e) => toast.error("Save failed:" + (e as Error).message))
      .finally(() => setSavingKey(false));
  };

  const saveTavilyKey = () => {
    setSavingTavilyKey(true);
    api
      .setSettings({ tavily_search_api_key: tavilyKeyInput })
      .then((s) => {
        apply(s);
        setTavilyKeyInput("");
        toast.success("Tavily API Key saved");
      })
      .catch((e) => toast.error("Save failed:" + (e as Error).message))
      .finally(() => setSavingTavilyKey(false));
  };

  const saveProxy = () => {
    setSavingProxy(true);
    api
      .setSettings({ web_search_proxy: proxyInput.trim() })
      .then((s) => {
        apply(s);
        toast.success(proxyInput.trim() ? "Export agent saved" : "The export proxy has been cleared (changed to direct connection)");
      })
      .catch((e) => toast.error("Save failed:" + (e as Error).message))
      .finally(() => setSavingProxy(false));
  };

  const saveGlobalProxy = () => {
    setSavingGlobalProxy(true);
    api
      .setSettings({ global_proxy: globalProxyInput.trim() })
      .then((s) => {
        apply(s);
        toast.success(globalProxyInput.trim() ? "Global agent saved" : "Global proxy cleared (changed to direct connection)");
      })
      .catch((e) => toast.error("Save failed:" + (e as Error).message))
      .finally(() => setSavingGlobalProxy(false));
  };

  // Run a real "test" search ("test") against the CURRENT form values (backend +
  // proxy + entered key), falling back to saved values server-side. Toasts result.
  const runTest = () => {
    setTesting(true);
    api
      .testWebSearch({
        web_search_backend: backend,
        web_search_proxy: proxyInput.trim(),
        brave_search_api_key: braveKeyInput,
        tavily_search_api_key: tavilyKeyInput,
      })
      .then((r) => {
        if (r.ok) toast.success(`Search test successful ·${r.backend}return${r.count}results`);
        else toast.error("Search test failed:" + (r.error || "Unknown error"));
      })
      .catch((e) => toast.error("Search test failed:" + (e as Error).message))
      .finally(() => setTesting(false));
  };

  // brave-free selected but no key stored and none being entered → tool stays off.
  const braveNeedsKey = webSearch && backend === "brave-free" && !braveKeySet;

  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">System configuration</h1>
        <p className="text-muted-foreground text-sm">Global runtime switch</p>
      </div>

      {/* Multiple columns instead of grid:The web search card is several times taller than the rest, and the height changes with the selected backend(brave/tavily
          of key Input is conditional rendering).grid Will fill the entire row with the highest one, leaving a large blank space next to it,
          Multiple columns are automatically filled in a balanced manner according to the height of the content. card spacing mb instead of gap——Under multi-column layout
          column-gap Only the column spacing, the row spacing must be set by the child elements themselves. */}
      <div className="columns-1 gap-4 md:gap-6 lg:columns-2">
        <UpdateCard />

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Traffic capture
            </CardTitle>
            <CardDescription>
              After being turned on, all Agent's HTTP traffic will be logged into the database through the recording agent, and traffic_search / traffic_get will be injected into the Agent.
              Tool and agent configuration (prompt word contains agent description).
              <br />
              Do not log any traffic when off (default): Agent
              <b>No</b>Get the proxy configuration and traffic tools, and the prompt words are also<b>Not included</b>Agency related content. After switching, the Agent will be rebuilt immediately to take effect.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="traffic-capture" className="text-sm font-normal text-muted-foreground">
              {trafficCapture ? "Enabled · Recording traffic and injecting proxy" : "Closed · No logging, no proxy injection"}
            </Label>
            <Switch
              id="traffic-capture"
              checked={trafficCapture}
              disabled={!loaded || saving}
              onCheckedChange={toggleTraffic}
            />
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Automatically link traffic to findings
            </CardTitle>
            <CardDescription id="agent-traffic-binding-description">
              The report writer can review captured HTTP requests and responses, link relevant records to a new finding, and use them in its report.
              <b> Reviewing traffic uses additional model tokens.</b>
              <br />
              Findings remain reportable when no matching traffic is available. This setting does not change traffic capture or manual evidence links. Agent runs use the updated setting on their next round.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="agent-traffic-binding" className="text-sm font-normal text-muted-foreground">
              {agentTrafficBinding ? "Enabled · Additional model usage" : "Disabled · Manual links remain available"}
            </Label>
            <Switch
              id="agent-traffic-binding"
              aria-describedby="agent-traffic-binding-description"
              checked={agentTrafficBinding}
              disabled={!loaded || saving}
              onCheckedChange={toggleAgentTrafficBinding}
            />
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Global agent
            </CardTitle>
            <CardDescription>
              All Agent's<b>Target traffic</b>Exit the network through this proxy (hide the source IP/take the springboard). support <b>http / https / socks5</b>, can be brought{" "}
              <code>user:pass</code> Certification. Leave blank = direct connection.
              <br />
              Open<b>Traffic capture</b>, it acts as a proxy of record<b>Upstream</b>(All traffic is still dropped into the database, and then goes out of the network through this proxy); when the capture is turned off, it is injected directly
              Agent's bash/WebFetch goes out of the network. Independent from the network search agent and LLM agent.
              <br />
              <b>Tips</b>:socks5 in<b>Turn off capture</b>relies on each command line tool pair <code>ALL_PROXY</code> support (curl
              available, some tools may ignore it); if you mainly use socks5, it is recommended to enable traffic capture - this path is provided by MITM
              Dial in person, the tool is imperceptible and works stably.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            <Label htmlFor="global-proxy" className="text-sm font-normal text-muted-foreground">
              Agent address
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id="global-proxy"
                autoComplete="off"
                placeholder="socks5://user:pass@host:1080 or http://host:port (leave blank = direct connection)"
                value={globalProxyInput}
                disabled={!loaded || savingGlobalProxy}
                onChange={(e) => setGlobalProxyInput(e.target.value)}
              />
              <Button type="button" onClick={saveGlobalProxy} disabled={!loaded || savingGlobalProxy}>
                Save
              </Button>
            </div>
            <p className="text-muted-foreground text-xs">
              {globalProxyInput.trim() ? "Configured · All target traffic goes out through this proxy" : "Not configured · Target traffic is directly connected to the outbound network"}
            </p>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ShieldAlertIcon className="size-4" />
              Operation constraint injection
            </CardTitle>
            <CardDescription>
              After opening, change the<b>Operational constraints</b>(The allow/deny entry maintained in the "Operation Constraints" of the task overview) Enter the corresponding Agent
              System prompts are used to frame the exploration boundaries (such as "only test the current port" and "no blasting").
              <br />
              can be individually controlled to inject into <b>Planner</b>With <b>Executor (worker)</b>
              ; Both are enabled by default. The switch takes effect immediately (the next round of reading), and there is no need to rebuild the Agent. After closing, the Agent no longer sees the constraints.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="inject-planner" className="text-sm font-normal text-muted-foreground">
                Inject planner{injectPlanner ? "· Enabled" : "· Closed"}
              </Label>
              <Switch
                id="inject-planner"
                checked={injectPlanner}
                disabled={!loaded}
                onCheckedChange={toggleInjectPlanner}
              />
            </div>
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="inject-worker" className="text-sm font-normal text-muted-foreground">
                Inject executor (worker){injectWorker ? "· Enabled" : "· Closed"}
              </Label>
              <Switch
                id="inject-worker"
                checked={injectWorker}
                disabled={!loaded}
                onCheckedChange={toggleInjectWorker}
              />
            </div>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <FlaskConicalIcon className="size-4" />
              Experimental features
            </CardTitle>
            <CardDescription>
              The mechanism is still under verification and is turned off by default. It may change Agent behavior or affect stability, please enable it after understanding the impact.
              <br />
              <b>noa context compression</b>: Long conversation history is actively compressed by the model (norma v0.4.0). The four types of Agents (
              <b>Planner/Executor/Main Agent/Dialogue</b>) use noa to take over the context instead of built-in compression,
              The compressed original text will be archived in the task working directory for easy backtracking. The switch takes effect immediately (effective for subsequent runs), and there is no need to rebuild the Agent;
              Built-in compression is restored immediately after shutdown.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="noa-compaction" className="text-sm font-normal text-muted-foreground">
              noa context compression{noaCompaction ? "· Enabled" : "· Closed"}
            </Label>
            <Switch
              id="noa-compaction"
              checked={noaCompaction}
              disabled={!loaded}
              onCheckedChange={toggleNoaCompaction}
            />
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <SearchIcon className="size-4" />
              Web search
            </CardTitle>
            <CardDescription>
              This is a web search<b>Master switch + source configuration</b>. After turning it on, you can<b>Configuration of each Agent</b>Individually choose whether to enable
              <b>web_search</b>(Only title/link/abstract is returned, the text is not fetched; fetching is done by WebFetch). web search<b>Not leaving</b>
              Logging agent, independent of traffic capture.
              <br />
              Source optional <b>DuckDuckGo(ddgs)</b>(No Key required),<b>Brave (free version)</b>(Brave API Key required),{" "}
              <b>Tavily</b>(Tavily API Key required) or <b>DeepSeek</b>(Reuse current LLM configuration). When the main switch is turned off, each
              The Agent's network search switch is unavailable.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="web-search" className="text-sm font-normal text-muted-foreground">
                {webSearch ? "The main switch is turned on · Can be enabled individually in each Agent configuration" : "Close · Each Agent cannot enable network search"}
              </Label>
              <Switch
                id="web-search"
                checked={webSearch}
                disabled={!loaded || saving}
                onCheckedChange={(v) => {
                  setWebSearch(v); // optimistic
                  saveWebSearch({ web_search_enabled: v });
                }}
              />
            </div>

            {webSearch && (
              <div className="flex items-center justify-between gap-4">
                <Label className="text-sm font-normal text-muted-foreground">Search source</Label>
                <Select
                  value={backend}
                  disabled={!loaded || saving}
                  onValueChange={(v) => {
                    setBackend(v); // optimistic
                    saveWebSearch({ web_search_backend: v });
                  }}
                >
                  <SelectTrigger className="w-48 shrink-0">
                    <SelectValue placeholder="Select source" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ddgs">DuckDuckGo (ddgs · Free No Key)</SelectItem>
                    <SelectItem value="brave-free">Brave (Free version · Key required)</SelectItem>
                    <SelectItem value="tavily">Tavily (Key required)</SelectItem>
                    <SelectItem value="deepseek">DeepSeek (official)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}

            {webSearch && backend === "deepseek" && (
              <div className="border-border/60 bg-muted/30 flex flex-col gap-2 rounded-md border p-3">
                <p className="text-sm font-medium">DeepSeek official Internet search</p>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  This source is reused directly<b>Currently activated LLM configuration</b>. therefore it
                  <b>Only supports DeepSeek official models</b>, and this configuration<b>Anthropic protocol must be used</b>
                   DeepSeek's OpenAI protocol endpoint does not support server-side search. This source may become invalid after switching the LLM configuration.
                </p>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  Unlike other sources, search by <b>DeepSeek server execution</b>: Each search will consume an additional model call (generating Token
                  fee), search request<b>Do not go through the above export agent</b>, also<b>Not included in traffic traces</b>;return result<b>Title and link only</b>
                  (no abstract), fetched by WebFetch when text is required.
                </p>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  It is up to you to confirm whether the above conditions are met, and the system will not intercept it; you can use the "Test Search" button below to actually run it to verify.
                </p>
              </div>
            )}

            {webSearch && backend === "brave-free" && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="brave-key" className="text-sm font-normal text-muted-foreground">
                  Brave Search API Key
                  {braveKeySet && <span className="ml-2 text-xs text-emerald-500">Configured</span>}
                </Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="brave-key"
                    type="password"
                    autoComplete="off"
                    placeholder={braveKeySet ? "Configured (leave blank to leave unchanged)" : "Enter Brave API Key"}
                    value={braveKeyInput}
                    disabled={!loaded || savingKey}
                    onChange={(e) => setBraveKeyInput(e.target.value)}
                  />
                  <Button
                    type="button"
                    onClick={saveBraveKey}
                    disabled={!loaded || savingKey || braveKeyInput.trim() === ""}
                  >
                    Save
                  </Button>
                </div>
                {braveNeedsKey && (
                  <p className="text-xs text-amber-500">
                    Brave is selected but the Key has not been configured - the search tool will not be enabled until the Key is saved.
                  </p>
                )}
                <p className="text-muted-foreground text-xs">
                  The free version has a limit of about 2,000 times/month. Go to https://brave.com/search/api/ to get the Key.
                </p>
              </div>
            )}

            {webSearch && backend === "tavily" && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="tavily-key" className="text-sm font-normal text-muted-foreground">
                  Tavily Search API Key
                  {tavilyKeySet && <span className="ml-2 text-xs text-emerald-500">Configured</span>}
                </Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="tavily-key"
                    type="password"
                    autoComplete="off"
                    placeholder={tavilyKeySet ? "Configured (leave blank to leave unchanged)" : "Enter Tavily API Key (tvly-…)"}
                    value={tavilyKeyInput}
                    disabled={!loaded || savingTavilyKey}
                    onChange={(e) => setTavilyKeyInput(e.target.value)}
                  />
                  <Button
                    type="button"
                    onClick={saveTavilyKey}
                    disabled={!loaded || savingTavilyKey || tavilyKeyInput.trim() === ""}
                  >
                    Save
                  </Button>
                </div>
                {webSearch && backend === "tavily" && !tavilyKeySet && (
                  <p className="text-xs text-amber-500">
                    Tavily is selected but the Key has not been configured - the search tool will not be enabled until the Key is saved.
                  </p>
                )}
                <p className="text-muted-foreground text-xs">Go to https://tavily.com to register and obtain API Key.</p>
              </div>
            )}

            {webSearch && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="ws-proxy" className="text-sm font-normal text-muted-foreground">
                  Export agent (optional)
                </Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="ws-proxy"
                    autoComplete="off"
                    placeholder="http://host:port or socks5://host:port (leave blank = direct connection)"
                    value={proxyInput}
                    disabled={!loaded || savingProxy}
                    onChange={(e) => setProxyInput(e.target.value)}
                  />
                  <Button type="button" onClick={saveProxy} disabled={!loaded || savingProxy}>
                    Save
                  </Button>
                </div>
                <p className="text-muted-foreground text-xs">
                  Separate egress proxy for accessing search endpoints only (VPN/SOCKS, etc.). It has nothing to do with the MITM proxy that records traffic; it is accessed through this proxy when the network is unavailable.
                </p>
              </div>
            )}

            {webSearch && (
              <div className="flex items-center justify-between gap-4 border-t pt-4">
                <p className="text-muted-foreground text-xs">
                  Use the current configuration (source + agent + key) to actually search for "test" to verify whether it is available.
                </p>
                <Button
                  type="button"
                  variant="outline"
                  onClick={runTest}
                  disabled={!loaded || testing}
                  className="shrink-0"
                >
                  {testing ? "Testing…" : "Test search"}
                </Button>
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Custom script · Python interpreter
            </CardTitle>
            <CardDescription>
              Customized <b>script</b> Type tools use it to run Python. It will be automatically detected when booting (python3 is preferred); venv / can be filled in manually here
              The absolute path to a specific version. If left blank, it will be automatically detected at runtime.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <Input
                className="font-mono text-sm"
                placeholder="/usr/bin/python3 (leave blank = auto-detect)"
                value={pyInterp}
                disabled={!loaded || saving}
                onChange={(e) => setPyInterp(e.target.value)}
              />
              <Button variant="outline" onClick={detectPython} disabled={!loaded || saving}>
                Retest
              </Button>
              <Button onClick={savePython} disabled={!loaded || saving}>
                Save
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <CpuIcon className="size-4" />
              Work concurrency·Number of Work Agents
            </CardTitle>
            <CardDescription>
              The number of worker agents to run concurrently for each task (default 3). The larger the value, the more concurrent probes and the higher the consumption. After modification
              <b>Effective for tasks started later</b>, running tasks are not affected.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <Input
                type="number"
                min={1}
                className="w-32 font-mono text-sm"
                placeholder="3"
                value={workers}
                disabled={!loaded || savingWorkers}
                onChange={(e) => setWorkers(e.target.value)}
              />
              <Button onClick={saveWorkers} disabled={!loaded || savingWorkers}>
                Save
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeyboardIcon className="size-4" />
              Conversation input box sends key position
            </CardTitle>
            <CardDescription>
              This setting is shared between the dialog page and the main Agent session input box of task details. It takes effect immediately after selection and does not need to be saved.
              <br />
              This preference<b>Only exists in this browser</b>, does not synchronize with the account, and needs to be reset after changing browsers or clearing site data.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="chat-send-mode" className="text-sm font-normal text-muted-foreground">
              Sending method
            </Label>
            <Select value={sendMode} onValueChange={(v) => setChatSendMode(v as ChatSendMode)}>
              <SelectTrigger id="chat-send-mode" className="w-72">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CHAT_SEND_MODE_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
