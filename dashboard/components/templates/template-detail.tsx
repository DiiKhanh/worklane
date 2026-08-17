"use client";

import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowLeft, FlaskConical, Save } from "lucide-react";
import type { Template, TemplateSend } from "@/lib/roadmap/templates";
import { TEMPLATE_VARS } from "@/lib/roadmap/templates";
import { renderPreview } from "@/lib/roadmap/template-render";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { DataTable } from "@/components/common/data-table";

const sendColumns: ColumnDef<TemplateSend>[] = [
  {
    accessorKey: "id",
    header: "Request",
    cell: ({ row }) => (
      <span className="group/row inline-flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{row.original.id}</span>
        <CopyButton value={row.original.id} label="Copy request id" />
      </span>
    ),
  },
  {
    accessorKey: "recipient",
    header: "Recipient",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] text-muted-foreground">
        {row.original.recipient}
      </span>
    ),
  },
  {
    accessorKey: "state",
    header: "State",
    cell: ({ row }) => <StateBadge state={row.original.state} />,
  },
  {
    accessorKey: "when",
    header: "Sent",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground tabular-nums">
        {row.original.when}
      </span>
    ),
  },
];

function PreviewCard({ subject, body, channel }: { subject: string; body: string; channel: "email" | "sms" }) {
  const rendered = renderPreview({ subject, body });
  return (
    <div className="mt-3 grid gap-2.5 rounded-xl border border-border bg-muted/40 p-4">
      <span className="w-fit text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
        {channel}
      </span>
      {channel === "email" && (
        <div className="text-sm font-medium">{rendered.subject}</div>
      )}
      <div className="whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
        {rendered.body}
      </div>
    </div>
  );
}

export function TemplateDetail({
  tpl,
  onBack,
}: {
  tpl: Template;
  onBack: () => void;
}) {
  const [subject, setSubject] = useState(tpl.subject);
  const [body, setBody] = useState(tpl.body);

  return (
    <div>
      <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
        <ArrowLeft className="size-4" />
        Template studio
      </Button>
      <SectionHeading
        title={tpl.name}
        description={tpl.id}
        action={
          <div className="flex gap-2">
            <Button size="sm" variant="outline">
              <FlaskConical className="size-3.5" />
              Test send
            </Button>
            <Button size="sm">
              <Save className="size-3.5" />
              Save version
            </Button>
          </div>
        }
      />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Badge variant="secondary">{tpl.channel}</Badge>
        <span className="font-mono text-[11px] text-muted-foreground">
          locale {tpl.locale}
        </span>
        <span className="font-mono text-[11px] text-muted-foreground">
          v{tpl.version}
        </span>
        <StateBadge state={tpl.status === "active" ? "active" : "expired"} />
        <span className="ml-auto text-xs text-muted-foreground">
          Updated {tpl.updated}
        </span>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Panel title="Editor" description="Variables resolve at render time">
          <div className="grid gap-4">
            {tpl.channel === "email" && (
              <div className="grid gap-1.5">
                <Label htmlFor="tpl-subject">Subject</Label>
                <Input
                  id="tpl-subject"
                  value={subject}
                  onChange={(e) => setSubject(e.target.value)}
                />
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
                  onClick={() => setBody((b) => `${b} ${v}`)}
                  className="rounded-full border border-border px-2 py-0.5 font-mono text-xs text-muted-foreground transition-colors hover:text-foreground"
                >
                  {v}
                </button>
              ))}
            </div>
          </div>
        </Panel>

        <Panel title="Preview" description="Rendered through the delivery path">
          <PreviewCard subject={subject} body={body} channel={tpl.channel} />
        </Panel>
      </div>

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <Panel title="Version history" description={`v${tpl.version} is live`}>
          <ul className="m-0 list-none p-0">
            {tpl.versions.map((h, i) => (
              <li
                key={h.v}
                className={`flex gap-3 py-2.5 ${i ? "border-t border-border/70" : ""}`}
              >
                <span className="w-7 shrink-0 font-mono text-[13px]">v{h.v}</span>
                <span className="flex-1 text-sm">
                  {h.note}
                  <span className="block text-xs text-muted-foreground">
                    {h.by} · {h.when}
                  </span>
                </span>
                {i === 0 ? (
                  <StateBadge state="active" />
                ) : (
                  <Button size="sm" variant="ghost">
                    Restore
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </Panel>

        <Panel
          title="Recent sends"
          description="Deliveries rendered from this template"
        >
          {tpl.sends.length ? (
            <DataTable
              columns={sendColumns}
              data={tpl.sends}
              pageSize={5}
              rowKey={(r) => r.id}
            />
          ) : (
            <div className="py-6 text-center text-sm text-muted-foreground">
              No sends from this template yet.
            </div>
          )}
        </Panel>
      </div>
    </div>
  );
}
