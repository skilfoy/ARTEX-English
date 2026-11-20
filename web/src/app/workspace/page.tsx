"use client";

import { createAuthClient } from "@neon/auth/next";
import { ArrowLeft, LockKeyhole, Plus } from "lucide-react";
import Link from "next/link";
import { type FormEvent, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  EMPTY_DRAFT,
  KINDS,
  type RecordDraft,
  type RecordKind,
  SEVERITIES,
  STATUSES,
  type Workspace,
  type WorkspaceRecord,
} from "@/lib/private-workspace/model";

const auth = createAuthClient();
const selectStyle =
  "h-9 rounded-lg border border-input bg-background px-2 text-sm focus-visible:outline-2 focus-visible:outline-ring";

async function workspaceRequest<T>(query = "", body?: object): Promise<T> {
  const response = await fetch(
    `/api/private-workspace${query}`,
    body
      ? {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }
      : { cache: "no-store" },
  );
  const result = await response.json();
  if (!response.ok) throw new Error(result.error ?? "The workspace request failed.");
  return result;
}

function Problem({ message }: { message: string }) {
  return message ? (
    <p role="alert" className="text-destructive text-sm">
      {message}
    </p>
  ) : null;
}

function SignIn() {
  const [step, setStep] = useState<"sign-in" | "sign-up" | "verify" | "forgot" | "reset">("sign-in");
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setNotice("");
    setBusy(true);
    try {
      const address = email.trim();
      if (step === "forgot") {
        const result = await auth.emailOtp.requestPasswordReset({ email: address });
        if (result.error) throw new Error("Could not request a reset code. Try again later.");
        setStep("reset");
        return;
      }
      if (step === "reset") {
        const result = await auth.emailOtp.resetPassword({ email: address, otp: code.trim(), password });
        if (result.error) throw new Error(result.error.message);
        setStep("sign-in");
        setCode("");
        setPassword("");
        setNotice("Password updated. Sign in with your new password.");
        return;
      }
      if (step === "sign-up") {
        const result = await auth.signUp.email({ name: name.trim(), email: address, password });
        if (result.error) throw new Error(result.error.message);
        setStep("verify");
        setPassword("");
        return;
      }
      if (step === "verify") {
        const result = await auth.emailOtp.verifyEmail({ email: address, otp: code.trim() });
        if (result.error) throw new Error(result.error.message);
        if (!result.data?.token) {
          setStep("sign-in");
          setNotice("Email verified. Sign in to open your workspace.");
        }
        setCode("");
        return;
      }
      const result = await auth.signIn.email({ email: address, password });
      if (result.error?.code === "EMAIL_NOT_VERIFIED") {
        setPassword("");
        setStep("verify");
        await resendCode(address);
      } else if (result.error) {
        throw new Error(result.error.message);
      }
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : "Could not complete sign-in. Try again.");
    } finally {
      setBusy(false);
    }
  }

  async function resendCode(address: string) {
    const { error } = await auth.emailOtp.sendVerificationOtp({ email: address, type: "email-verification" });
    if (error) throw new Error(error.message);
    setNotice("Check your email for a new verification code.");
  }

  const heading = {
    "sign-in": "Sign in",
    "sign-up": "Create an account",
    verify: "Verify your email",
    forgot: "Request a password reset",
    reset: "Reset your password",
  }[step];
  const submitLabel = {
    "sign-in": "Sign in",
    "sign-up": "Create account",
    verify: "Verify email",
    forgot: "Send reset code",
    reset: "Reset password",
  }[step];
  let description = "Your workspaces and records are private to your account.";
  if (step === "verify") description = `Enter the verification code sent to ${email.trim()}.`;
  if (step === "reset") description = "If that address has an account, check its inbox for a reset code.";
  const needsCode = step === "verify" || step === "reset";
  const newPassword = step === "sign-up" || step === "reset";

  return (
    <section className="max-w-md rounded-xl border bg-card p-5 sm:p-6" aria-labelledby="workspace-sign-in">
      <h2 id="workspace-sign-in" className="font-semibold text-lg">
        {heading}
      </h2>
      <p className="mt-1 text-muted-foreground text-sm">{description}</p>
      <form onSubmit={submit} className="mt-5 grid gap-4">
        {step === "sign-up" && (
          <label htmlFor="account-name" className="grid gap-1.5 text-sm">
            Name
            <Input
              id="account-name"
              autoComplete="name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              required
              maxLength={100}
            />
          </label>
        )}
        {!needsCode && (
          <label htmlFor="account-email" className="grid gap-1.5 text-sm">
            Email
            <Input
              id="account-email"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              required
            />
          </label>
        )}
        {needsCode && (
          <label htmlFor="account-code" className="grid gap-1.5 text-sm">
            {step === "verify" ? "Verification code" : "Reset code"}
            <Input
              id="account-code"
              autoComplete="one-time-code"
              inputMode="numeric"
              value={code}
              onChange={(event) => setCode(event.target.value)}
              required
              maxLength={16}
            />
          </label>
        )}
        {(step === "sign-in" || newPassword) && (
          <label htmlFor="account-password" className="grid gap-1.5 text-sm">
            {newPassword ? "New password" : "Password"}
            <Input
              id="account-password"
              type="password"
              autoComplete={newPassword ? "new-password" : "current-password"}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
              minLength={newPassword ? 12 : undefined}
              maxLength={128}
            />
          </label>
        )}
        {newPassword && <p className="text-muted-foreground text-sm">Use at least 12 characters.</p>}
        <Problem message={error} />
        {notice && (
          <p role="status" className="text-sm">
            {notice}
          </p>
        )}
        <Button type="submit" size="lg" disabled={busy}>
          {busy ? "Please wait" : submitLabel}
        </Button>
        {step === "sign-in" && (
          <Button
            variant="link"
            disabled={busy}
            onClick={() => {
              setStep("forgot");
              setError("");
              setNotice("");
              setPassword("");
            }}
          >
            Forgot password?
          </Button>
        )}
      </form>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button
          variant="link"
          onClick={() => {
            setStep(step === "sign-in" ? "sign-up" : "sign-in");
            setError("");
            setNotice("");
            setPassword("");
          }}
          disabled={busy}
        >
          {step === "sign-in" ? "Create an account" : "Back to sign in"}
        </Button>
        {step === "verify" && (
          <Button
            variant="link"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                await resendCode(email.trim());
              } catch (err) {
                setError(err instanceof Error ? err.message : "Could not resend the code.");
              } finally {
                setBusy(false);
              }
            }}
          >
            Resend code
          </Button>
        )}
      </div>
    </section>
  );
}

function Records({ workspace }: { workspace: Workspace }) {
  const [records, setRecords] = useState<WorkspaceRecord[] | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [draft, setDraft] = useState<RecordDraft>(EMPTY_DRAFT);
  const [editing, setEditing] = useState<string>();
  const [view, setView] = useState<{ page: number; kindFilter: RecordKind | "all" }>({ page: 1, kindFilter: "all" });
  const { page, kindFilter } = view;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState("");

  useEffect(() => {
    let active = true;
    const params = new URLSearchParams({ workspace: workspace.id, page: String(view.page) });
    if (view.kindFilter !== "all") params.set("kind", view.kindFilter);
    workspaceRequest<{ records: WorkspaceRecord[]; hasMore: boolean }>(`?${params}`)
      .then((result) => {
        if (active) {
          setRecords(result.records);
          setHasMore(result.hasMore);
        }
      })
      .catch((err) => {
        if (active) setError(err.message);
      });
    return () => {
      active = false;
    };
  }, [workspace.id, view]);

  function browse(nextPage: number, nextKind: RecordKind | "all") {
    setRecords(null);
    setHasMore(false);
    setError("");
    setView({ page: nextPage, kindFilter: nextKind });
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || !records) return;
    setBusy(true);
    setSaved("");
    setError("");
    try {
      await workspaceRequest<{ record: WorkspaceRecord }>("", {
        ...draft,
        action: "saveRecord",
        workspace: workspace.id,
        id: editing,
      });
      browse(1, "all");
      setSaved(editing ? "Record updated." : "Record added.");
      setDraft(EMPTY_DRAFT);
      setEditing(undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save this record.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section aria-label="Workspace records" className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(290px,340px)]">
      <div className="min-w-0 rounded-xl border bg-card p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="break-all font-semibold text-xl">{workspace.name}</h2>
          <label className="flex items-center gap-2 text-sm">
            Filter
            <select
              className={selectStyle}
              value={kindFilter}
              disabled={busy}
              onChange={(event) => browse(1, event.target.value as RecordKind | "all")}
            >
              <option value="all">All records</option>
              {Object.entries(KINDS).map(([kind, label]) => (
                <option key={kind} value={kind}>
                  {label}
                </option>
              ))}
            </select>
          </label>
        </div>
        {records === null && !error && (
          <p role="status" className="mt-5 text-sm">
            Loading records…
          </p>
        )}
        {records?.length === 0 && (
          <p className="mt-5 text-muted-foreground text-sm">
            {kindFilter !== "all" || page > 1
              ? "No records match this view."
              : "This workspace is empty. Add your first record to get started."}
          </p>
        )}
        <ul className="mt-4 divide-y">
          {records?.map((item) => (
            <li key={item.id} className="flex items-start justify-between gap-3 py-3">
              <div className="min-w-0 break-words">
                <h3 className="font-medium">{item.title}</h3>
                <p className="text-muted-foreground text-sm">
                  {[KINDS[item.kind], item.status, item.severity].filter(Boolean).join(" · ")}
                </p>
                {item.details && <p className="mt-1 whitespace-pre-wrap break-words text-sm">{item.details}</p>}
              </div>
              <Button
                variant="outline"
                aria-label={`Edit ${item.title}`}
                disabled={busy}
                onClick={() => {
                  setEditing(item.id);
                  setDraft({
                    kind: item.kind,
                    title: item.title,
                    details: item.details,
                    severity: item.severity,
                    status: item.status,
                  });
                  setSaved("");
                }}
              >
                Edit
              </Button>
            </li>
          ))}
        </ul>
        {(page > 1 || hasMore) && (
          <nav aria-label="Record pages" className="mt-3 flex items-center gap-3 text-sm">
            <Button variant="outline" disabled={busy || page === 1} onClick={() => browse(page - 1, kindFilter)}>
              Previous
            </Button>
            <span>Page {page}</span>
            <Button variant="outline" disabled={busy || !hasMore} onClick={() => browse(page + 1, kindFilter)}>
              Next
            </Button>
          </nav>
        )}
      </div>
      <form
        onSubmit={save}
        className="grid gap-3 self-start rounded-xl border bg-card p-5"
        aria-label={editing ? "Edit record" : "Add record"}
      >
        <h2 className="font-semibold">{editing ? "Edit record" : "Add record"}</h2>
        <label className="grid gap-1 text-sm">
          Record type
          <select
            className={selectStyle}
            value={draft.kind}
            onChange={(event) => setDraft({ ...draft, kind: event.target.value as RecordKind, severity: null })}
          >
            <option value="asset">Asset</option>
            <option value="finding">Finding</option>
            <option value="evidence">Evidence</option>
            <option value="remediation">Remediation</option>
          </select>
        </label>
        <label htmlFor="record-title" className="grid gap-1 text-sm">
          Title
          <Input
            id="record-title"
            value={draft.title}
            onChange={(event) => setDraft({ ...draft, title: event.target.value })}
            required
            maxLength={160}
          />
        </label>
        <label className="grid gap-1 text-sm">
          Status
          <select
            className={selectStyle}
            value={draft.status}
            onChange={(event) => setDraft({ ...draft, status: event.target.value as RecordDraft["status"] })}
          >
            {STATUSES.map((status) => (
              <option key={status} value={status}>
                {status.charAt(0).toUpperCase() + status.slice(1)}
              </option>
            ))}
          </select>
        </label>
        {draft.kind === "finding" && (
          <label className="grid gap-1 text-sm">
            Severity
            <select
              className={selectStyle}
              value={draft.severity ?? ""}
              onChange={(event) =>
                setDraft({ ...draft, severity: (event.target.value || null) as RecordDraft["severity"] })
              }
            >
              <option value="">Unrated</option>
              {SEVERITIES.map((severity) => (
                <option key={severity} value={severity}>
                  {severity}
                </option>
              ))}
            </select>
          </label>
        )}
        <label htmlFor="record-details" className="grid gap-1 text-sm">
          Details
          <Textarea
            id="record-details"
            rows={4}
            value={draft.details}
            onChange={(event) => setDraft({ ...draft, details: event.target.value })}
            maxLength={12000}
            placeholder="Notes or evidence references"
          />
        </label>
        <Problem message={error} />
        {saved && (
          <p role="status" className="text-sm">
            {saved}
          </p>
        )}
        <div className="flex gap-2">
          <Button type="submit" disabled={busy || records === null}>
            {busy ? "Saving…" : "Save record"}
          </Button>
          {editing && (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => {
                setEditing(undefined);
                setDraft(EMPTY_DRAFT);
              }}
            >
              Cancel
            </Button>
          )}
        </div>
      </form>
    </section>
  );
}

function WorkspacePicker() {
  const [workspaces, setWorkspaces] = useState<Workspace[] | null>(null);
  const [selected, setSelected] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    workspaceRequest<{ workspaces: Workspace[] }>()
      .then(({ workspaces }) => {
        if (active) {
          setWorkspaces(workspaces);
          setSelected(workspaces[0]?.id ?? "");
        }
      })
      .catch((err) => {
        if (active) setError(err.message);
      });
    return () => {
      active = false;
    };
  }, []);

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || !workspaces) return;
    setBusy(true);
    setError("");
    try {
      const { workspace } = await workspaceRequest<{ workspace: Workspace }>("", { action: "createWorkspace", name });
      setWorkspaces([workspace, ...workspaces]);
      setSelected(workspace.id);
      setName("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create the workspace.");
    } finally {
      setBusy(false);
    }
  }

  const workspace = workspaces?.find((item) => item.id === selected);
  return (
    <div className="grid gap-5">
      <div className="flex flex-wrap items-end gap-4 rounded-xl border bg-card p-5">
        {!!workspaces?.length && (
          <label className="grid min-w-48 gap-1.5 text-sm">
            Open workspace
            <select value={selected} onChange={(event) => setSelected(event.target.value)} className={selectStyle}>
              {workspaces.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <form onSubmit={create} className="flex flex-wrap items-end gap-2">
          <label htmlFor="workspace-name" className="grid gap-1.5 text-sm">
            New workspace name
            <Input
              id="workspace-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={100}
              required
            />
          </label>
          <Button type="submit" disabled={busy || workspaces === null}>
            <Plus aria-hidden="true" />
            {busy ? "Creating…" : "Create workspace"}
          </Button>
        </form>
      </div>
      <Problem message={error} />
      {workspaces === null && !error && (
        <p role="status" className="text-sm">
          Loading workspaces…
        </p>
      )}
      {workspaces?.length === 0 && (
        <p className="text-muted-foreground text-sm">Create a workspace to start with a blank set of records.</p>
      )}
      {workspace && <Records key={workspace.id} workspace={workspace} />}
    </div>
  );
}

function AccountWorkspace() {
  const { data, isPending } = auth.useSession();
  const user = data?.user?.emailVerified ? data.user : null;
  const [error, setError] = useState("");
  return (
    <>
      {user && (
        <div className="flex flex-wrap items-center gap-3 text-sm">
          <span className="break-all text-muted-foreground">{user.email}</span>
          <Button
            variant="outline"
            onClick={async () => {
              try {
                const { error } = await auth.signOut();
                if (error) setError("Could not sign out. Try again.");
              } catch {
                setError("Could not sign out. Try again.");
              }
            }}
          >
            Sign out
          </Button>
        </div>
      )}
      <Problem message={error} />
      {isPending && <p role="status">Checking sign-in…</p>}
      {!isPending && (user ? <WorkspacePicker key={user.id} /> : <SignIn />)}
    </>
  );
}

export default function WorkspacePage() {
  return (
    <main className="mx-auto grid max-w-5xl gap-5 px-4 py-8 sm:px-6 sm:py-12">
      <Link
        href="/function/tasks"
        className="inline-flex w-fit items-center gap-2 font-medium text-primary text-sm hover:underline"
      >
        <ArrowLeft aria-hidden="true" className="size-4" /> Back to interactive demo
      </Link>
      <header>
        <h1 className="flex items-center gap-2 font-semibold text-2xl">
          <LockKeyhole aria-hidden="true" className="size-5" /> Private workspace
        </h1>
        <p className="mt-2 max-w-2xl text-muted-foreground text-sm">
          Create an empty workspace for assets, findings, evidence, and remediation. Records you add are saved to your
          account.
        </p>
      </header>
      {process.env.NEXT_PUBLIC_PRIVATE_WORKSPACE === "1" ? (
        <AccountWorkspace />
      ) : (
        <p className="text-sm">Private workspace sign-in is unavailable on this deployment.</p>
      )}
    </main>
  );
}
