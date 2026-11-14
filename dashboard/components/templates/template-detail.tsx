"use client";

import { useEffect, useState } from "react";
import { ArrowLeft, LoaderCircle, Save } from "lucide-react";
import type { Template, TemplateVersion } from "@/lib/api/types";
import {
  useAddVersion,
  usePreview,
  usePublishVersion,
  useTemplate,
} from "@/lib/queries/use-templates";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { StateBadge } from "@/components/common/state-badge";
import { Skeleton } from "@/components/ui/skeleton";

// Only variables wired end-to-end in the OTP flow. {{name}}/{{link}} arrive with later
// sub-projects (B/C); the backend rejects any other token at author time.
const TEMPLATE_VARS = ["{{code}}", "{{expiry}}"];

export function TemplateDetail({ template, onBack }: { template: Template; onBack: () => void }) {
  const { data, isLoading, isError, refetch } = useTemplate(template.id);
  const publish = usePublishVersion(template.id);
  const [dirty, setDirty] = useState(false);

  const versions = data?.versions ?? [];
  const activeId = data?.template.activeVersionId ?? template.activeVersionId;
  // Seed the editor from the live version, or the newest draft for a never-published one.
  const seed = versions.find((v) => v.id === activeId) ?? versions[0];

  return (
    <div>
      <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
        <ArrowLeft className="size-4" />
        Template studio
      </Button>
      <SectionHeading title={template.name} description={template.id} />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Badge variant="secondary">{template.channel}</Badge>
        <span className="font-mono text-[11px] text-muted-foreground">locale {template.locale}</span>
        <StateBadge state={template.status === "active" ? "active" : "expired"} />
      </div>

      {isLoading ? (
        <Skeleton className="h-80 rounded-xl" />
      ) : isError && !data ? (
        <div role="alert" className="flex items-center gap-3 py-4 text-sm text-destructive">
          Could not load this template.
          <Button variant="outline" size="sm" onClick={() => refetch()}>Retry</Button>
        </div>
      ) : (
        <Editor
          key={activeId || "draft"}
          template={template}
          seed={seed}
          onDirtyChange={setDirty}
          disabled={publish.isPending}
        />
      )}

      <div className="mt-4">
        <Panel
          title="Version history"
          description="Publish a version to make it live; publish an older one to roll back"
        >
          <p role="status" className="mb-3 text-xs text-muted-foreground">
            {publish.isPending
              ? "Publishing version..."
              : dirty
                ? "Save the current changes before making a version live."
                : publish.isSuccess
                  ? "Version is live. New sends will use this version."
                  : "Saved drafts do not affect delivery until made live."}
          </p>
          {publish.isError && (
            <p role="alert" className="mb-3 text-sm text-destructive">
              Could not publish this version. Try again.
            </p>
          )}
          <ul className="m-0 list-none p-0">
            {versions.map((v, i) => {
              const isActive = v.id === activeId;
              return (
                <li key={v.id} className={`flex items-center gap-3 py-2.5 ${i ? "border-t border-border/70" : ""}`}>
                  <span className="w-7 shrink-0 font-mono text-[13px]">v{v.versionNo}</span>
                  <span className="min-w-0 flex-1 break-words text-sm">
                    {v.note || "(no note)"}
                    <span className="block text-xs text-muted-foreground">
                      {v.createdBy} · {v.status}
                    </span>
                  </span>
                  {isActive ? (
                    <StateBadge state="active" />
                  ) : (
                    <Button
                      size="sm"
                      variant="outline"
                      aria-label={`Make v${v.versionNo} live`}
                      onClick={() => publish.mutate(v.id)}
                      disabled={publish.isPending || dirty}
                    >
                      {publish.isPending && publish.variables === v.id && (
                        <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" />
                      )}
                      {publish.isPending && publish.variables === v.id ? "Publishing..." : "Make live"}
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        </Panel>
      </div>
    </div>
  );
}

// Publishing reseeds the editor; saving another draft keeps its content and feedback.
function Editor({ template, seed, onDirtyChange, disabled }: {
  template: Template;
  seed?: TemplateVersion;
  onDirtyChange: (dirty: boolean) => void;
  disabled: boolean;
}) {
  const addVersion = useAddVersion(template.id);
  const { mutate: runPreview } = usePreview();

  const [subject, setSubject] = useState(seed?.subject ?? "");
  const [body, setBody] = useState(seed?.body ?? "");
  const [note, setNote] = useState("");
  const [saved, setSaved] = useState({ subject: seed?.subject ?? "", body: seed?.body ?? "" });
  const [preview, setPreview] = useState<{
    subject: string;
    body: string;
    rendered?: { subject: string; body: string };
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const dirty = subject !== saved.subject || body !== saved.body || !!note.trim();
  const previewCurrent = preview?.subject === subject && preview?.body === body;
  const rendered = previewCurrent ? preview.rendered : undefined;

  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange]);

  // Live preview through the real render path (debounced), so it cannot diverge from what
  // the dispatcher sends. setState happens inside the mutation callback, not the effect body.
  useEffect(() => {
    let current = true;
    const timer = setTimeout(() => {
      runPreview(
        { channel: template.channel, subject, body },
        {
          onSuccess: (rendered) => {
            if (current) setPreview({ subject, body, rendered });
          },
          onError: () => {
            if (current) setPreview({ subject, body });
          },
        },
      );
    }, 250);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  }, [subject, body, template.channel, runPreview]);

  async function saveDraft() {
    setError(null);
    try {
      await addVersion.mutateAsync({ subject, body, note: note.trim() });
      setSaved({ subject, body });
      setNote("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save version");
    }
  }

  return (
    <>
      <div className="mb-3 flex items-center justify-between gap-3">
        <p role="status" className="text-xs text-muted-foreground">
          {addVersion.isPending
            ? "Saving draft..."
            : dirty
              ? "Unsaved changes"
              : addVersion.isSuccess
                ? `Draft v${addVersion.data.versionNo} saved`
                : seed ? `Editing v${seed.versionNo}` : "New draft"}
        </p>
        <Button
          size="sm"
          onClick={saveDraft}
          disabled={disabled || addVersion.isPending || !dirty || !body.trim() || (template.channel === "email" && !subject.trim())}
        >
          {addVersion.isPending
            ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" />
            : <Save className="size-3.5" />}
          {addVersion.isPending ? "Saving..." : "Save draft"}
        </Button>
      </div>
      {error && <p role="alert" className="mb-3 text-sm text-destructive">{error}</p>}

      <div className="grid gap-4 md:grid-cols-2">
        <Panel title="Editor" description="Variables resolve at render time">
          <fieldset disabled={disabled || addVersion.isPending} className="grid min-w-0 gap-4">
            {template.channel === "email" && (
              <div className="grid gap-1.5">
                <Label htmlFor="tpl-subject">Subject</Label>
                <Input id="tpl-subject" value={subject} onChange={(e) => setSubject(e.target.value)} />
              </div>
            )}
            <div className="grid gap-1.5">
              <Label htmlFor="tpl-body">Body</Label>
              <Textarea
                id="tpl-body"
                rows={6}
                value={body}
                onChange={(e) => setBody(e.target.value)}
                className="font-mono text-[13px]"
              />
            </div>
            <div className="flex flex-wrap gap-1.5">
              {TEMPLATE_VARS.map((v) => (
                <button
                  key={v}
                  type="button"
                  onClick={() => setBody((b) => `${b}${v}`)}
                  className="rounded-full border border-border px-2 py-0.5 font-mono text-xs text-muted-foreground transition-colors hover:text-foreground"
                >
                  {v}
                </button>
              ))}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="tpl-note">Change note</Label>
              <Input
                id="tpl-note"
                placeholder="What changed in this version?"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>
          </fieldset>
        </Panel>

        <Panel title="Preview" description="Rendered through the delivery path">
          <div aria-busy={!previewCurrent} className="mt-1 grid min-h-32 gap-2.5 rounded-xl border border-border bg-muted/40 p-4">
            <span className="w-fit text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
              {template.channel}
            </span>
            {!previewCurrent ? (
              <p role="status" className="text-xs text-muted-foreground">Updating preview...</p>
            ) : !rendered ? (
              <p role="alert" className="text-sm text-destructive">Preview unavailable. Check the content or connection and edit to retry.</p>
            ) : (
              <>
                {template.channel === "email" && <div className="break-words text-sm font-medium">{rendered.subject}</div>}
                <div className="whitespace-pre-wrap break-words text-sm leading-relaxed text-muted-foreground">
                  {rendered.body}
                </div>
              </>
            )}
          </div>
        </Panel>
      </div>
    </>
  );
}
