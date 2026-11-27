"use client";

import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { ArrowLeft, Ban, CalendarDays, ExternalLink, MousePointerClick, RotateCw, TrendingUp } from "lucide-react";
import { ApiError } from "@/lib/api/errors";
import type { Link, LinkAnalytics, LinkVisit } from "@/lib/api/types";
import { timeAgo } from "@/lib/format";
import { useLink, useLinkAnalytics } from "@/lib/queries/use-links";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Panel } from "@/components/common/panel";
import { StatCard } from "@/components/common/stat-card";
import { StateBadge } from "@/components/common/state-badge";
import { DataTable } from "@/components/common/data-table";
import { ClicksArea, dayLabel } from "./clicks-area";
import { CopyLinkButton } from "./copy-link-button";
import { DisableLinkDialog } from "./disable-link-dialog";
import { Unavailable } from "./unavailable";

export function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString("en-US", { dateStyle: "medium", timeStyle: "short" });
}

const visitColumns: ColumnDef<LinkVisit>[] = [
  {
    accessorKey: "ts",
    header: "When",
    enableSorting: false,
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground tabular-nums" title={formatDateTime(row.original.ts)}>
        {timeAgo(row.original.ts)}
      </span>
    ),
  },
  {
    accessorKey: "referrerOrigin",
    header: "Referrer",
    enableSorting: false,
    cell: ({ row }) =>
      row.original.referrerOrigin ? (
        <span className="font-mono text-[13px]">{row.original.referrerOrigin}</span>
      ) : (
        <span className="text-sm text-muted-foreground">Unknown</span>
      ),
  },
  {
    accessorKey: "device",
    header: "Device",
    enableSorting: false,
    cell: ({ row }) => (
      <span className="text-sm capitalize text-muted-foreground">{row.original.device}</span>
    ),
  },
];

export function LinkDetail({
  code,
  initial,
  onBack,
}: {
  code: string;
  initial?: Link;
  onBack: () => void;
}) {
  const meta = useLink(code, initial);
  const [confirming, setConfirming] = useState(false);
  const link = meta.data;

  return (
    <div>
      <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
        <ArrowLeft className="size-4" />
        Links
      </Button>

      {link ? (
        <>
          {/* SectionHeading styling, but URLs may be 4 KB without spaces and must wrap. */}
          <div className="mb-5 min-w-0 space-y-1">
            <h2 className="text-xl font-semibold tracking-tight break-all">{link.shortUrl}</h2>
            <p className="line-clamp-3 text-sm break-all text-muted-foreground" title={link.longUrl}>
              {link.longUrl}
            </p>
          </div>
          <div className="-mt-2 mb-5 flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
              <StateBadge state={link.state} />
              <span>Created {formatDateTime(link.createdAt)}</span>
              {link.disabledAt && (
                <span>
                  Disabled {formatDateTime(link.disabledAt)} - visits get 410 Gone; history is kept
                </span>
              )}
            </div>
            <div className="flex flex-wrap gap-2">
              <CopyLinkButton value={link.shortUrl} />
              <Button
                size="sm"
                variant="outline"
                nativeButton={false}
                render={<a href={link.longUrl} target="_blank" rel="noreferrer" />}
              >
                <ExternalLink className="size-3.5" />
                Open destination
              </Button>
              {link.state === "active" && (
                <Button size="sm" variant="destructive" onClick={() => setConfirming(true)}>
                  <Ban className="size-3.5" />
                  Disable
                </Button>
              )}
            </div>
          </div>
          <DisableLinkDialog link={link} open={confirming} onClose={() => setConfirming(false)} />
        </>
      ) : meta.isError ? (
        <div className="mb-5">
          <Unavailable
            title={meta.error instanceof ApiError && meta.error.status === 404 ? "Link not found" : "Link details unavailable"}
            description={
              meta.error instanceof ApiError && meta.error.status === 404
                ? "No link with this code belongs to your tenant."
                : "The link service did not respond. Analytics below may still load."
            }
            onRetry={() => void meta.refetch()}
            retrying={meta.isFetching}
          />
        </div>
      ) : (
        <div className="mb-5 grid gap-2" aria-busy="true">
          <Skeleton className="h-7 w-72" />
          <Skeleton className="h-4 w-96 max-w-full" />
        </div>
      )}

      {!(meta.error instanceof ApiError && meta.error.status === 404) && <AnalyticsSection code={code} />}
    </div>
  );
}

function AnalyticsSection({ code }: { code: string }) {
  const analytics = useLinkAnalytics(code);
  const { data } = analytics;

  return (
    <section aria-labelledby="link-analytics-heading" className="grid grid-cols-1 gap-4">
      <div className="flex items-center justify-between gap-3">
        <div className="space-y-0.5">
          <h3 id="link-analytics-heading" className="text-sm font-medium">
            Click analytics
          </h3>
          <p className="m-0 text-xs text-muted-foreground">
            Recorded redirects, including repeats, bots and link previews - not unique visitors.
            New clicks usually appear within 10 seconds.
          </p>
        </div>
        <Button
          size="sm"
          variant="outline"
          onClick={() => void analytics.refetch()}
          disabled={analytics.isFetching}
          className="shrink-0"
        >
          <RotateCw className={analytics.isFetching ? "size-3.5 animate-spin motion-reduce:animate-none" : "size-3.5"} />
          {analytics.isFetching ? "Refreshing…" : "Refresh"}
        </Button>
      </div>

      {data ? (
        <AnalyticsBody data={data} />
      ) : analytics.isError ? (
        <Unavailable
          title="Analytics unavailable"
          description="Click counts could not be loaded. The link itself still works and can be copied."
          onRetry={() => void analytics.refetch()}
          retrying={analytics.isFetching}
        />
      ) : (
        <div className="grid gap-4" aria-busy="true">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Skeleton className="h-[104px] rounded-xl" />
            <Skeleton className="h-[104px] rounded-xl" />
            <Skeleton className="h-[104px] rounded-xl" />
          </div>
          <Skeleton className="h-[250px] rounded-xl" />
        </div>
      )}
      {data && analytics.isRefetchError && (
        <p role="alert" className="m-0 text-xs text-[var(--state-failed)]">
          Refresh failed - showing the last loaded analytics.
        </p>
      )}
    </section>
  );
}

function AnalyticsBody({ data }: { data: LinkAnalytics }) {
  const recentTotal = data.series.reduce((sum, d) => sum + d.clicks, 0);
  const peak = data.series.reduce<{ date: string; clicks: number } | null>(
    (best, d) => (d.clicks > 0 && (!best || d.clicks > best.clicks) ? d : best),
    null,
  );

  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard label="Total clicks" icon={MousePointerClick} accent="var(--state-verified)" hint="lifetime">
          {data.totalClicks.toLocaleString("en-US")}
        </StatCard>
        <StatCard label="Last 14 days" icon={TrendingUp} accent="var(--state-sent)" hint="UTC days, including today">
          {recentTotal.toLocaleString("en-US")}
        </StatCard>
        <StatCard
          label="Busiest day"
          icon={CalendarDays}
          accent="var(--state-requested)"
          hint={peak ? `${dayLabel(peak.date)} (UTC)` : "no clicks in the last 14 days"}
        >
          {peak ? peak.clicks.toLocaleString("en-US") : "-"}
        </StatCard>
      </div>

      <Panel title="Clicks over time" description="Daily totals, last 14 UTC days">
        <ClicksArea series={data.series} />
      </Panel>

      <Panel title="Recent visits" description="Newest first; details are kept for 30 days">
        {data.recent.length ? (
          <DataTable columns={visitColumns} data={data.recent} pageSize={20} />
        ) : (
          <p className="m-0 py-6 text-center text-sm text-muted-foreground">
            {data.totalClicks > 0
              ? "No visits in the last 30 days. Older visits still count toward the totals."
              : "No visits recorded yet. Open the short link to record one."}
          </p>
        )}
      </Panel>
    </>
  );
}
