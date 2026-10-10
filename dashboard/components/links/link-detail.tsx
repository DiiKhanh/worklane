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
import type { LinkClick } from "@/lib/api/types";
import { useLink } from "@/lib/queries/use-links";
import { stripScheme, timeAgo } from "@/lib/format";
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
        {timeAgo(row.original.ts)}
      </span>
    ),
  },
  {
    accessorKey: "ref",
    header: "Source",
    cell: ({ row }) => <span className="text-sm">{row.original.ref || "Direct"}</span>,
  },
  {
    accessorKey: "geo",
    header: "Location",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground">{row.original.geo || "-"}</span>
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

export function LinkDetail({ code, onBack }: { code: string; onBack: () => void }) {
  const { data: link, isLoading, error } = useLink(code);

  const back = (
    <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
      <ArrowLeft className="size-4" />
      Links
    </Button>
  );

  if (isLoading || !link) {
    return (
      <div>
        {back}
        {error ? (
          <p role="alert" className="py-10 text-center text-sm text-destructive">
            Could not load this link. {error.message}
          </p>
        ) : (
          <div className="py-10 text-center text-sm text-muted-foreground">Loading link…</div>
        )}
      </div>
    );
  }

  const peak = Math.max(0, ...link.series);
  const last14 = link.series.reduce((sum, v) => sum + v, 0);

  async function copyLink(shortUrl: string) {
    try {
      await navigator.clipboard.writeText(shortUrl);
    } catch {
      // Clipboard can be unavailable (insecure context); fail quietly.
    }
  }

  return (
    <div>
      {back}
      <SectionHeading
        title={stripScheme(link.shortUrl)}
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
            <Button size="sm" variant="outline" onClick={() => copyLink(link.shortUrl)}>
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
          <StatCard label="Last 14 days" icon={TrendingUp} hint="clicks in the window">
            <CountUp value={last14} />
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
          <DataTable columns={clickColumns} data={link.recent} pageSize={8} />
        </Panel>
      </div>
    </div>
  );
}
