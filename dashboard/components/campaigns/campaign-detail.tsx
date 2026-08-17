"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  ArrowLeft,
  CheckCheck,
  Copy,
  MailOpen,
  MousePointerClick,
  Pause,
  Users,
} from "lucide-react";
import type { Campaign } from "@/lib/roadmap/campaigns";
import type { OtpState } from "@/lib/api/types";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { StatCard } from "@/components/common/stat-card";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { DataTable } from "@/components/common/data-table";
import { CampaignStatus } from "./campaign-status";

const nf = (n: number) => n.toLocaleString("en-US");

type Recipient = { id: string; to: string; state: OtpState; opened: string };

const RECIPIENTS: Recipient[] = [
  { id: "snd_a91c", to: "d***@gmail.com", state: "verified", opened: "3m ago" },
  { id: "snd_7f20", to: "m***@outlook.com", state: "sent", opened: "-" },
  { id: "snd_45be", to: "k***@worklane.io", state: "verified", opened: "12m ago" },
  { id: "snd_0c83", to: "s***@proton.me", state: "failed", opened: "-" },
];

const columns: ColumnDef<Recipient>[] = [
  {
    accessorKey: "id",
    header: "Send",
    cell: ({ row }) => (
      <span className="group/row inline-flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{row.original.id}</span>
        <CopyButton value={row.original.id} label="Copy send id" />
      </span>
    ),
  },
  {
    accessorKey: "to",
    header: "Recipient",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] text-muted-foreground">
        {row.original.to}
      </span>
    ),
  },
  {
    accessorKey: "state",
    header: "State",
    cell: ({ row }) => <StateBadge state={row.original.state} />,
  },
  {
    accessorKey: "opened",
    header: "Opened",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground tabular-nums">
        {row.original.opened}
      </span>
    ),
  },
];

export function CampaignDetail({
  cmp,
  onBack,
}: {
  cmp: Campaign;
  onBack: () => void;
}) {
  const pct = (n: number) =>
    cmp.recipients ? `${Math.round((n / cmp.recipients) * 100)}%` : "-";
  const running = cmp.status === "sending" || cmp.status === "scheduled";
  return (
    <div>
      <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
        <ArrowLeft className="size-4" />
        Campaigns
      </Button>
      <SectionHeading
        title={cmp.name}
        description={cmp.id}
        action={
          <div className="flex gap-2">
            <Button size="sm" variant="outline">
              <Copy className="size-3.5" />
              Duplicate
            </Button>
            {running && (
              <Button size="sm" variant="destructive">
                <Pause className="size-3.5" />
                Pause
              </Button>
            )}
          </div>
        }
      />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Badge variant={cmp.kind === "otp" ? "outline" : "secondary"}>
          {cmp.kind}
        </Badge>
        <Badge variant="secondary">{cmp.channel}</Badge>
        <CampaignStatus status={cmp.status} />
        <span className="font-mono text-[11px] text-muted-foreground">
          {cmp.template}
        </span>
        <span className="ml-auto text-xs text-muted-foreground">
          {cmp.audience} · {cmp.when}
        </span>
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label="Recipients" icon={Users} hint={cmp.audience}>
          {nf(cmp.recipients)}
        </StatCard>
        <StatCard
          label="Delivered"
          icon={CheckCheck}
          accent="var(--state-verified)"
          hint={`${pct(cmp.delivered)} of scope`}
        >
          {nf(cmp.delivered)}
        </StatCard>
        <StatCard
          label="Opened"
          icon={MailOpen}
          accent="var(--chart-2)"
          hint={`${pct(cmp.opened)} of scope`}
        >
          {nf(cmp.opened)}
        </StatCard>
        <StatCard
          label="Clicked"
          icon={MousePointerClick}
          accent="var(--chart-3)"
          hint={`${pct(cmp.clicked)} of scope`}
        >
          {nf(cmp.clicked)}
        </StatCard>
      </div>

      <div className="mt-4">
        <Panel
          title="Recent sends"
          description="Per-recipient outcome from the delivery log"
        >
          <DataTable
            columns={columns}
            data={RECIPIENTS}
            pageSize={5}
            rowKey={(r) => r.id}
          />
        </Panel>
      </div>
    </div>
  );
}
