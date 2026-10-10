"use client";

import { useState, type FormEvent } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Link as LinkIcon, MousePointerClick } from "lucide-react";
import type { LinkSummary, ShortenResult } from "@/lib/api/types";
import { useCreateLink, useLinks } from "@/lib/queries/use-links";
import { stripScheme, timeAgo } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { StatCard } from "@/components/common/stat-card";
import { CountUp } from "@/components/common/count-up";
import { DataTable } from "@/components/common/data-table";
import { CopyButton } from "@/components/common/copy-button";
import { LinkDetail } from "./link-detail";

const columns: ColumnDef<LinkSummary>[] = [
  {
    accessorKey: "code",
    header: "Short link",
    cell: ({ row }) => (
      <span className="group/row inline-flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{stripScheme(row.original.shortUrl)}</span>
        <CopyButton value={row.original.shortUrl} label="Copy short link" />
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
    accessorKey: "createdAt",
    header: "Created",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground">{timeAgo(row.original.createdAt)}</span>
    ),
  },
];

export function LinksView() {
  const { data: rows = [], isLoading, error: loadError } = useLinks();
  const create = useCreateLink();
  const [open, setOpen] = useState<string | null>(null);
  const [longUrl, setLongUrl] = useState("");
  const [created, setCreated] = useState<ShortenResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (open) {
    return <LinkDetail key={open} code={open} onBack={() => setOpen(null)} />;
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const value = longUrl.trim();
    if (!value) return;
    setError(null);
    setCreated(null);
    try {
      setCreated(await create.mutateAsync(value));
      setLongUrl("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create the link");
    }
  }

  const totalClicks = rows.reduce((sum, l) => sum + l.clicks, 0);

  return (
    <div>
      <SectionHeading
        title="Links"
        description="Short links carried by notifications, and what they earn. Select a link for its clicks."
      />
      <div className="grid gap-4">
        <div className="grid grid-cols-2 gap-4">
          <StatCard
            label="Links"
            icon={LinkIcon}
            accent="var(--state-sent)"
            hint="most recent, this tenant"
          >
            <CountUp value={rows.length} />
          </StatCard>
          <StatCard
            label="Clicks"
            icon={MousePointerClick}
            accent="var(--state-verified)"
            hint="302 redirects served by these links"
          >
            <CountUp value={totalClicks} />
          </StatCard>
        </div>
        <Panel
          title="Shorten a URL"
          description="base62 over a distributed id - codes are never enumerable"
        >
          <form onSubmit={submit} className="flex gap-2">
            <Input
              aria-label="Long URL"
              placeholder="https://worklane.io/settings/notifications"
              value={longUrl}
              onChange={(e) => setLongUrl(e.target.value)}
            />
            <Button type="submit" className="shrink-0" disabled={create.isPending || !longUrl.trim()}>
              <LinkIcon className="size-4" />
              {create.isPending ? "Creating…" : "Create"}
            </Button>
          </form>
          {error && (
            <p role="alert" className="mt-2 text-sm text-destructive">
              {error}
            </p>
          )}
          {created && (
            <p className="group/row mt-2 inline-flex items-center gap-1.5 text-sm text-muted-foreground">
              Short link ready:
              <span className="font-mono text-[13px] text-foreground">
                {stripScheme(created.shortUrl)}
              </span>
              <CopyButton value={created.shortUrl} label="Copy short link" />
            </p>
          )}
          <div className="mt-4">
            {isLoading ? (
              <div className="py-10 text-center text-sm text-muted-foreground">Loading links…</div>
            ) : loadError ? (
              <p role="alert" className="py-10 text-center text-sm text-destructive">
                Could not load links. {loadError.message}
              </p>
            ) : (
              <DataTable
                columns={columns}
                data={rows}
                pageSize={10}
                rowKey={(l) => l.code}
                onRowClick={(l) => setOpen(l.code)}
              />
            )}
          </div>
        </Panel>
      </div>
    </div>
  );
}
