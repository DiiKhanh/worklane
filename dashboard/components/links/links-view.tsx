"use client";

import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Link as LinkIcon,
  MousePointerClick,
  TrendingUp,
  Zap,
} from "lucide-react";
import type { Link } from "@/lib/roadmap/links";
import { LINKS, LINK_KPIS } from "@/lib/roadmap/links";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SectionHeading } from "@/components/common/section-heading";
import { RoadmapNote } from "@/components/common/roadmap-note";
import { Panel } from "@/components/common/panel";
import { StatCard } from "@/components/common/stat-card";
import { CountUp } from "@/components/common/count-up";
import { DataTable } from "@/components/common/data-table";
import { CopyButton } from "@/components/common/copy-button";
import { LinkDetail } from "./link-detail";

const columns: ColumnDef<Link>[] = [
  {
    accessorKey: "code",
    header: "Short code",
    cell: ({ row }) => (
      <span className="group/row inline-flex items-center gap-1.5">
        <span className="font-mono text-[13px]">wl.link/{row.original.code}</span>
        <CopyButton
          value={`https://wl.link/${row.original.code}`}
          label="Copy short link"
        />
      </span>
    ),
  },
  {
    accessorKey: "target",
    header: "Target",
    cell: ({ row }) => (
      <span className="block max-w-[280px] truncate font-mono text-[13px] text-muted-foreground">
        {row.original.target}
      </span>
    ),
  },
  {
    accessorKey: "clicks",
    header: "Clicks",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] tabular-nums">
        {row.original.clicks.toLocaleString("en-US")}
      </span>
    ),
  },
  {
    accessorKey: "ctr",
    header: "CTR",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] tabular-nums text-muted-foreground">
        {Math.round(row.original.ctr * 100)}%
      </span>
    ),
  },
  {
    accessorKey: "created",
    header: "Created",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground">{row.original.created}</span>
    ),
  },
];

export function LinksView() {
  const [open, setOpen] = useState<Link | null>(null);
  if (open) {
    return <LinkDetail link={open} onBack={() => setOpen(null)} />;
  }

  return (
    <div>
      <SectionHeading
        title="Links"
        description="Short links carried by notifications, and what they earn. Select a link for its clicks."
      />
      <RoadmapNote />
      <div className="grid gap-4">
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            label="Links created"
            icon={LinkIcon}
            accent="var(--state-sent)"
            hint="last 30 days"
          >
            <CountUp value={LINK_KPIS.created} />
          </StatCard>
          <StatCard
            label="Clicks"
            icon={MousePointerClick}
            accent="var(--state-verified)"
            hint="302 redirects served"
          >
            <CountUp value={LINK_KPIS.clicks} />
          </StatCard>
          <StatCard
            label="Click rate"
            icon={TrendingUp}
            hint="of delivered messages"
          >
            <CountUp
              value={LINK_KPIS.clickRate}
              format={(v) => `${Math.round(v * 100)}%`}
            />
          </StatCard>
          <StatCard
            label="Cache hit"
            icon={Zap}
            accent="var(--state-requested)"
            hint="Redis cache-aside"
          >
            <CountUp
              value={LINK_KPIS.cacheHit}
              format={(v) => `${Math.round(v * 100)}%`}
            />
          </StatCard>
        </div>
        <Panel
          title="Shorten a URL"
          description="base62 over a distributed id - codes are never enumerable"
        >
          <div className="flex gap-2">
            <Input placeholder="https://worklane.io/settings/notifications" />
            <Button className="shrink-0">
              <LinkIcon className="size-4" />
              Create
            </Button>
          </div>
          <div className="mt-4">
            <DataTable
              columns={columns}
              data={LINKS}
              pageSize={10}
              rowKey={(l) => l.code}
              onRowClick={setOpen}
            />
          </div>
        </Panel>
      </div>
    </div>
  );
}
