"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { ListChecks, Search } from "lucide-react";
import type { OtpRequest, OtpState } from "@/lib/api/types";
import { useRequests } from "@/lib/queries/use-requests";
import { useUIStore } from "@/lib/store/ui";
import { DataTable } from "@/components/common/data-table";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { EmptyState } from "@/components/common/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";

const STATES: (OtpState | "all")[] = [
  "all",
  "requested",
  "sent",
  "verified",
  "failed",
  "expired",
];

const columns: ColumnDef<OtpRequest>[] = [
  {
    accessorKey: "id",
    header: "Request",
    cell: ({ row }) => (
      <div className="group/row flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{row.original.id}</span>
        <CopyButton value={row.original.id} label="Copy request id" />
      </div>
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
    accessorKey: "channel",
    header: "Channel",
    cell: ({ row }) => (
      <span className="text-sm capitalize text-muted-foreground">
        {row.original.channel}
      </span>
    ),
  },
  {
    accessorKey: "state",
    header: "State",
    cell: ({ row }) => <StateBadge state={row.original.state} />,
  },
  {
    accessorKey: "createdAt",
    header: "Created",
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground tabular-nums">
        {timeAgo(row.original.createdAt) || "-"}
      </span>
    ),
  },
];

export function RequestsView() {
  const { data, isLoading } = useRequests();
  const { filters, setFilter } = useUIStore();
  const activeState = filters.state ?? "all";
  const search = filters.search ?? "";

  const filtered = useMemo(() => {
    if (!data) return [];
    return data.filter((r) => {
      const stateOk = activeState === "all" || r.state === activeState;
      const searchOk =
        !search || r.recipient.toLowerCase().includes(search.toLowerCase());
      return stateOk && searchOk;
    });
  }, [data, activeState, search]);

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap gap-1.5">
          {STATES.map((s) => {
            const active = activeState === s;
            return (
              <button
                key={s}
                type="button"
                onClick={() => setFilter("state", s === "all" ? undefined : s)}
                className={cn(
                  "rounded-full border px-3 py-1 text-xs font-medium capitalize transition-colors",
                  active
                    ? "border-primary/40 bg-primary/12 text-foreground"
                    : "border-border text-muted-foreground hover:text-foreground",
                )}
              >
                {s}
              </button>
            );
          })}
        </div>
        <div className="relative sm:w-64">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setFilter("search", e.target.value || undefined)}
            placeholder="Search recipient"
            className="pl-8"
          />
        </div>
      </div>

      {isLoading || !data ? (
        <Skeleton className="h-96 rounded-xl" />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={ListChecks}
          title="No matching requests"
          description="Try a different state or search term."
        />
      ) : (
        <DataTable columns={columns} data={filtered} pageSize={12} />
      )}
    </div>
  );
}
