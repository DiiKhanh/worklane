"use client";

import { useEffect, useState } from "react";
import { ArrowLeft, Save } from "lucide-react";
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

// Only variables wired end-to-end in the OTP flow. {{name}}/{{link}} arrive with later
// sub-projects (B/C); the backend rejects any other token at author time.
const TEMPLATE_VARS = ["{{code}}", "{{expiry}}"];

export function TemplateDetail({ template, onBack }: { template: Template; onBack: () => void }) {
  const { data } = useTemplate(template.id);
  const publish = usePublishVersion(template.id);

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

      {/* Keyed by the seed version so switching the live version reseeds the editor. */}
      <Editor key={seed?.id ?? "empty"} template={template} seed={seed} />

      <div className="mt-4">
        <Panel
          title="Version history"
          description="Publish a version to make it live; publish an older one to roll back"
        >
          <ul className="m-0 list-none p-0">
            {versions.map((v, i) => {
              const isActive = v.id === activeId;
              return (
                <li key={v.id} className={`flex items-center gap-3 py-2.5 ${i ? "border-t border-border/70" : ""}`}>
                  <span className="w-7 shrink-0 font-mono text-[13px]">v{v.versionNo}</span>
                  <span className="flex-1 text-sm">
                    {v.note || "(no note)"}
                    <span className="block text-xs text-muted-foreground">
                      {v.createdBy} · {v.status}
                    </span>
                  </span>
                  {isActive ? (
                    <StateBadge state="active" />
                  ) : (
                    <Button size="sm" variant="ghost" onClick={() => publish.mutate(v.id)} disabled={publish.isPending}>
                      Make live
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

// Editor holds the draft-in-progress. It is keyed by the seed version, so React remounts
// it (reseeding useState from props) whenever the live version changes - no seeding effect.
function Editor({ template, seed }: { template: Template; seed?: TemplateVersion }) {
  const addVersion = useAddVersion(template.id);
  const { mutate: runPreview } = usePreview();

  const [subject, setSubject] = useState(seed?.subject ?? "");
  const [body, setBody] = useState(seed?.body ?? "");
  const [note, setNote] = useState("");
  const [rendered, setRendered] = useState({ subject: seed?.subject ?? "", body: seed?.body ?? "" });
  const [error, setError] = useState<string | null>(null);

  // Live preview through the real render path (debounced), so it cannot diverge from what
  // the dispatcher sends. setState happens inside the mutation callback, not the effect body.
  useEffect(() => {
    const timer = setTimeout(() => {
      runPreview(
        { channel: template.channel, subject, body },
        { onSuccess: setRendered, onError: () => setRendered({ subject, body }) },
      );
    }, 250);
    return () => clearTimeout(timer);
  }, [subject, body, template.channel, runPreview]);

  async function saveDraft() {
    setError(null);
    try {
      await addVersion.mutateAsync({ subject, body, note: note.trim() });
      setNote("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save version");
    }
  }

  return (
    <>
      <div className="mb-3 flex justify-end">
        <Button size="sm" onClick={saveDraft} disabled={addVersion.isPending}>
          <Save className="size-3.5" />
          Save draft
        </Button>
      </div>
      {error && <p className="mb-3 text-sm text-destructive">{error}</p>}

      <div className="grid gap-4 md:grid-cols-2">
        <Panel title="Editor" description="Variables resolve at render time">
          <div className="grid gap-4">
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
          </div>
        </Panel>

        <Panel title="Preview" description="Rendered through the delivery path">
          <div className="mt-1 grid gap-2.5 rounded-xl border border-border bg-muted/40 p-4">
            <span className="w-fit text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
              {template.channel}
            </span>
            {template.channel === "email" && <div className="text-sm font-medium">{rendered.subject}</div>}
            <div className="whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
              {rendered.body}
            </div>
          </div>
        </Panel>
      </div>
    </>
  );
}
