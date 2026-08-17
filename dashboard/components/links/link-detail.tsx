"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  ArrowLeft,
  Calendar,
  Copy,
  CornerUpRight,
  ExternalLink,
  MousePointerClick,
  TrendingUp,
} from "lucide-react";
import type { Link, LinkClick } from "@/lib/roadmap/links";
import { Button } from "@/components/ui/button";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { StatCard } from "@/components/common/stat-card";
import { CountUp } from "@/components/common/count-up";
import { DataTable } from "@/components/common/data-table";
import { ClicksArea } from "./clicks-area";

const clickColumns: ColumnDef<LinkClick>[] = [
  {
    accessorKey: "ts",
    header: "When",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground tabular-nums">
        {row.original.ts}
      </span>
    ),
  },
  {
    accessorKey: "ref",
    header: "Source",
    cell: ({ row }) => <span className="text-sm">{row.original.ref}</span>,
  },
  {
    accessorKey: "geo",
    header: "Location",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground">{row.original.geo}</span>
    ),
  },
  {
    accessorKey: "device",
    header: "Device",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] text-muted-foreground">
        {row.original.device}
      </span>
    ),
  },
];

export function LinkDetail({ link, onBack }: { link: Link; onBack: () => void }) {
  const peak = Math.max(...link.series);
  return (
    <div>
      <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
        <ArrowLeft className="size-4" />
        Links
      </Button>
      <SectionHeading
        title={`wl.link/${link.code}`}
        description={link.target}
        action={
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              render={
                <a href={link.target} target="_blank" rel="noreferrer" />
              }
            >
              <ExternalLink className="size-3.5" />
              Open target
            </Button>
            <Button size="sm" variant="outline">
              <Copy className="size-3.5" />
              Copy link
            </Button>
          </div>
        }
      />

      <div className="grid gap-4">
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            label="Total clicks"
            icon={MousePointerClick}
            accent="var(--state-verified)"
            hint="all time"
          >
            <CountUp value={link.clicks} />
          </StatCard>
          <StatCard label="CTR" icon={TrendingUp} hint="of messages carrying it">
            <CountUp value={link.ctr} format={(v) => `${Math.round(v * 100)}%`} />
          </StatCard>
          <StatCard
            label="Peak day"
            icon={Calendar}
            accent="var(--state-sent)"
            hint="clicks in a day"
          >
            <CountUp value={peak} />
          </StatCard>
          <StatCard
            label="Redirect"
            icon={CornerUpRight}
            accent="var(--state-requested)"
            hint="keeps clicks trackable"
          >
            302
          </StatCard>
        </div>

        <Panel title="Clicks over time" description="Last 14 days">
          <ClicksArea series={link.series} />
        </Panel>

        <Panel
          title="Recent clicks"
          description="Append-only click events emit link.clicked to analytics"
        >
          <DataTable
            columns={clickColumns}
            data={link.recent}
            pageSize={8}
            rowKey={(c) => c.ts + c.geo}
          />
        </Panel>
      </div>
    </div>
  );
}
