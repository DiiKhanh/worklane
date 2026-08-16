"use client";

import { useRequests } from "@/lib/queries/use-requests";
import { StateBadge } from "@/components/common/state-badge";
import { Skeleton } from "@/components/ui/skeleton";
import { timeAgo } from "@/lib/format";

export function ActivityFeed({ limit = 7 }: { limit?: number }) {
  const { data, isLoading } = useRequests();

  if (isLoading || !data) {
    return (
      <div className="space-y-3">
        {Array.from({ length: limit }).map((_, i) => (
          <Skeleton key={i} className="h-8 rounded-md" />
        ))}
      </div>
    );
  }

  const rows = data.slice(0, limit);

  return (
    <ul className="divide-y divide-border/70">
      {rows.map((r) => (
        <li key={r.id} className="flex items-center gap-3 py-2.5 first:pt-0 last:pb-0">
          <StateBadge state={r.state} />
          <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground">
            {r.recipient}
          </span>
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {timeAgo(r.createdAt)}
          </span>
        </li>
      ))}
    </ul>
  );
}
