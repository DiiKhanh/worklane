"use client";

import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Bell } from "lucide-react";
import type { Notification, NotificationState } from "@/lib/api/types";
import { useNotifications } from "@/lib/queries/use-notifications";
import { DataTable } from "@/components/common/data-table";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { EmptyState } from "@/components/common/empty-state";
import { LiveDot } from "@/components/common/live-dot";
import { NotificationDetail } from "@/components/notifications/notification-detail";
import { SectionHeading } from "@/components/common/section-heading";
import { Skeleton } from "@/components/ui/skeleton";
import { timeAgo } from "@/lib/format";
import { cn } from "@/lib/utils";

const STATES: (NotificationState | "all")[] = [
  "all",
  "queued",
  "sent",
  "failed",
  "suppressed",
];

const columns: ColumnDef<Notification>[] = [
  {
    accessorKey: "id",
    header: "Notification",
    cell: ({ row }) => (
      <div className="group/row flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{row.original.id}</span>
        <CopyButton value={row.original.id} label="Copy notification id" />
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
    accessorKey: "kind",
    header: "Kind",
    cell: ({ row }) => (
      <span className="text-sm capitalize text-muted-foreground">
        {row.original.kind}
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

export function NotificationsView() {
  const { data, isLoading, error } = useNotifications();
  const [activeState, setActiveState] = useState<NotificationState | "all">("all");
  const [openId, setOpenId] = useState<string | null>(null);

  const filtered = useMemo(
    () => (data ?? []).filter((n) => activeState === "all" || n.state === activeState),
    [data, activeState],
  );

  if (openId) {
    return <NotificationDetail id={openId} onBack={() => setOpenId(null)} />;
  }

  return (
    <div>
      <SectionHeading
        title="Notifications"
        description="Every notification accepted for your tenant. Recipients are masked at rest. Select one for its delivery and engagement."
        action={
          <span className="inline-flex items-center gap-2 rounded-full border border-border bg-card/60 px-3 py-1 text-xs text-muted-foreground">
            <LiveDot />
            Live · every 4s
          </span>
        }
      />
      <div className="mb-4 flex flex-wrap gap-1.5">
        {STATES.map((s) => {
          const active = activeState === s;
          return (
            <button
              key={s}
              type="button"
              aria-pressed={active}
              onClick={() => setActiveState(s)}
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

      {error && !data ? (
        <p role="alert" className="py-10 text-center text-sm text-destructive">
          Could not load notifications. {error.message}
        </p>
      ) : isLoading || !data ? (
        <Skeleton className="h-96 rounded-xl" />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={Bell}
          title={data.length === 0 ? "No notifications yet" : "No matching notifications"}
          description={
            data.length === 0
              ? "Notifications sent through the API appear here."
              : "Try a different state."
          }
        />
      ) : (
        <DataTable
          columns={columns}
          data={filtered}
          pageSize={12}
          rowKey={(n) => n.id}
          onRowClick={(n) => setOpenId(n.id)}
        />
      )}
    </div>
  );
}
