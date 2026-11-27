"use client";

import { useLayoutEffect, useRef, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Link as LinkIcon } from "lucide-react";
import type { Link } from "@/lib/api/types";
import { timeAgo } from "@/lib/format";
import { useLinkPage } from "@/lib/queries/use-links";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { DataTable } from "@/components/common/data-table";
import { CopyButton } from "@/components/common/copy-button";
import { EmptyState } from "@/components/common/empty-state";
import { StateBadge } from "@/components/common/state-badge";
import { CreateLinkForm } from "./create-link-form";
import { LinkDetail, formatDateTime } from "./link-detail";
import { Unavailable } from "./unavailable";

// link-svc's default page size; the API, not the table, paginates.
const PAGE_SIZE = 20;

const columns: ColumnDef<Link>[] = [
  {
    accessorKey: "shortUrl",
    header: "Short link",
    enableSorting: false,
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{row.original.shortUrl}</span>
        <CopyButton value={row.original.shortUrl} label={`Copy ${row.original.shortUrl}`} />
      </span>
    ),
  },
  {
    accessorKey: "longUrl",
    header: "Destination",
    enableSorting: false,
    cell: ({ row }) => (
      <span
        title={row.original.longUrl}
        className="block max-w-[320px] truncate font-mono text-[13px] text-muted-foreground"
      >
        {row.original.longUrl}
      </span>
    ),
  },
  {
    accessorKey: "state",
    header: "State",
    enableSorting: false,
    cell: ({ row }) => <StateBadge state={row.original.state} />,
  },
  {
    accessorKey: "createdAt",
    header: "Created",
    enableSorting: false,
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground" title={formatDateTime(row.original.createdAt)}>
        {timeAgo(row.original.createdAt)}
      </span>
    ),
  },
];

type Selection = { code: string; initial?: Link };

export function LinksView() {
  // Cursors of the pages visited so far; the last entry is the current page.
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [open, setOpen] = useState<Selection | null>(null);
  const page = useLinkPage(cursors[cursors.length - 1]);
  const detailRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLTableRowElement | null>(null);

  // Move focus into the detail on open and back to the activating row on close.
  useLayoutEffect(() => {
    if (open) detailRef.current?.querySelector("button")?.focus({ preventScroll: true });
    else if (triggerRef.current?.isConnected) triggerRef.current.focus({ preventScroll: true });
  }, [open]);

  const pageNumber = cursors.length;
  const items = page.data?.items ?? [];
  const nextCursor = page.data?.nextCursor ?? null;

  let list: React.ReactNode;
  if (page.data) {
    list =
      items.length === 0 && pageNumber === 1 ? (
        <EmptyState
          icon={LinkIcon}
          title="No links yet"
          description="Shorten a URL above to create your first link."
        />
      ) : (
        <>
          <DataTable
            columns={columns}
            data={items}
            pageSize={PAGE_SIZE}
            rowKey={(l) => l.code}
            onRowClick={(l, element) => {
              triggerRef.current = element;
              setOpen({ code: l.code, initial: l });
            }}
          />
          {(pageNumber > 1 || nextCursor) && (
            <nav
              aria-label="Links pagination"
              className="mt-3 flex items-center justify-between gap-4 text-sm text-muted-foreground"
            >
              <span className="tabular-nums">Page {pageNumber}</span>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCursors((c) => c.slice(0, -1))}
                  disabled={pageNumber === 1 || page.isFetching}
                >
                  Previous
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => nextCursor && setCursors((c) => [...c, nextCursor])}
                  disabled={!nextCursor || page.isFetching}
                >
                  Next
                </Button>
              </div>
            </nav>
          )}
        </>
      );
  } else if (page.isError) {
    list = (
      <Unavailable
        title="Links unavailable"
        description="Your links could not be loaded from the link service."
        onRetry={() => void page.refetch()}
        retrying={page.isFetching}
      />
    );
  } else {
    list = <Skeleton aria-busy="true" aria-label="Loading links" className="h-64 rounded-xl" />;
  }

  return (
    <>
      <div ref={detailRef} hidden={!open}>
        {open && (
          <LinkDetail
            key={open.code}
            code={open.code}
            initial={open.initial}
            onBack={() => setOpen(null)}
          />
        )}
      </div>
      <div hidden={!!open}>
        <SectionHeading
          title="Links"
          description="Shorten URLs, copy them, and see the clicks they record. Select a link for details."
        />
        {/* grid-cols-1 is minmax(0, 1fr): the wide table scrolls inside its panel. */}
        <div className="grid grid-cols-1 gap-4">
          <Panel title="Shorten a URL" description="Shortening the same URL again returns its existing link">
            <CreateLinkForm
              onCreated={() => setCursors([undefined])}
              onOpen={(code) => {
                triggerRef.current = null;
                setOpen({ code });
              }}
            />
          </Panel>
          <Panel title="Your links" description="Newest first">
            {list}
          </Panel>
        </div>
      </div>
    </>
  );
}
