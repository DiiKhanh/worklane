"use client";

import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import type { Template } from "@/lib/roadmap/templates";
import { TEMPLATES } from "@/lib/roadmap/templates";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SectionHeading } from "@/components/common/section-heading";
import { RoadmapNote } from "@/components/common/roadmap-note";
import { StateBadge } from "@/components/common/state-badge";
import { DataTable } from "@/components/common/data-table";
import { TemplateDetail } from "./template-detail";
import { NewTemplateDialog } from "./new-template-dialog";

const columns: ColumnDef<Template>[] = [
  {
    accessorKey: "name",
    header: "Template",
    cell: ({ row }) => (
      <span className="grid">
        <span className="font-medium">{row.original.name}</span>
        <span className="font-mono text-[11px] text-muted-foreground">
          {row.original.id}
        </span>
      </span>
    ),
  },
  {
    accessorKey: "channel",
    header: "Channel",
    cell: ({ row }) => <Badge variant="secondary">{row.original.channel}</Badge>,
  },
  {
    accessorKey: "locale",
    header: "Locale",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] text-muted-foreground">
        {row.original.locale}
      </span>
    ),
  },
  {
    accessorKey: "version",
    header: "Version",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] tabular-nums">
        v{row.original.version}
      </span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => (
      <StateBadge
        state={row.original.status === "active" ? "active" : "expired"}
      />
    ),
  },
  {
    accessorKey: "updated",
    header: "Updated",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground">{row.original.updated}</span>
    ),
  },
];

export function TemplatesView() {
  const [rows, setRows] = useState<Template[]>(TEMPLATES);
  const [open, setOpen] = useState<Template | null>(null);
  const [creating, setCreating] = useState(false);

  if (open) {
    return <TemplateDetail key={open.id} tpl={open} onBack={() => setOpen(null)} />;
  }

  return (
    <div>
      <SectionHeading
        title="Template studio"
        description="Author the message once; the dispatcher renders from it. Select a template to edit."
        action={
          <Button size="sm" onClick={() => setCreating(true)}>
            <Plus className="size-4" />
            New template
          </Button>
        }
      />
      <RoadmapNote />
      <DataTable
        columns={columns}
        data={rows}
        pageSize={10}
        rowKey={(t) => t.id}
        onRowClick={setOpen}
      />
      <NewTemplateDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreate={(t) => {
          setRows((r) => [t, ...r]);
          setCreating(false);
          setOpen(t);
        }}
      />
    </div>
  );
}
