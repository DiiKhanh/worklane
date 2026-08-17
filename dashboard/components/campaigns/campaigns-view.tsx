"use client";

import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import type { Campaign } from "@/lib/roadmap/campaigns";
import { CAMPAIGNS } from "@/lib/roadmap/campaigns";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SectionHeading } from "@/components/common/section-heading";
import { RoadmapNote } from "@/components/common/roadmap-note";
import { DataTable } from "@/components/common/data-table";
import { CampaignStatus } from "./campaign-status";
import { CampaignComposer, type QueuedCampaign } from "./campaign-composer";
import { CampaignDetail } from "./campaign-detail";

const nf = (n: number) => n.toLocaleString("en-US");

const columns: ColumnDef<Campaign>[] = [
  {
    accessorKey: "name",
    header: "Campaign",
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
    accessorKey: "kind",
    header: "Type",
    cell: ({ row }) => (
      <Badge variant={row.original.kind === "otp" ? "outline" : "secondary"}>
        {row.original.kind}
      </Badge>
    ),
  },
  {
    accessorKey: "channel",
    header: "Channel",
    cell: ({ row }) => <Badge variant="secondary">{row.original.channel}</Badge>,
  },
  {
    accessorKey: "audience",
    header: "Audience",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground">
        {row.original.audience}
      </span>
    ),
  },
  {
    accessorKey: "recipients",
    header: "Recipients",
    cell: ({ row }) => (
      <span className="tabular-nums">{nf(row.original.recipients)}</span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <CampaignStatus status={row.original.status} />,
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

type View =
  | { mode: "list" }
  | { mode: "new" }
  | { mode: "detail"; cmp: Campaign };

export function CampaignsView() {
  const [rows, setRows] = useState<Campaign[]>(CAMPAIGNS);
  const [view, setView] = useState<View>({ mode: "list" });

  if (view.mode === "new") {
    return (
      <CampaignComposer
        onBack={() => setView({ mode: "list" })}
        onQueue={(c: QueuedCampaign) => {
          const created: Campaign = {
            id: `cmp_${Math.random().toString(16).slice(2, 8)}`,
            delivered: 0,
            opened: 0,
            clicked: 0,
            ...c,
          };
          setRows((r) => [created, ...r]);
          setView({ mode: "detail", cmp: created });
        }}
      />
    );
  }

  if (view.mode === "detail") {
    return (
      <CampaignDetail cmp={view.cmp} onBack={() => setView({ mode: "list" })} />
    );
  }

  return (
    <div>
      <SectionHeading
        title="Campaigns"
        description="Send an OTP batch or a marketing message to a saved audience or an uploaded list."
        action={
          <Button size="sm" onClick={() => setView({ mode: "new" })}>
            <Plus className="size-4" />
            New campaign
          </Button>
        }
      />
      <RoadmapNote />
      <DataTable
        columns={columns}
        data={rows}
        pageSize={10}
        rowKey={(c) => c.id}
        onRowClick={(c) => setView({ mode: "detail", cmp: c })}
      />
    </div>
  );
}
