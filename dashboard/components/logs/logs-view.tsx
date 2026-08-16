"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Truck } from "lucide-react";
import type { DeliveryLog } from "@/lib/api/types";
import { useLogs } from "@/lib/queries/use-logs";
import { DataTable } from "@/components/common/data-table";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { EmptyState } from "@/components/common/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

const columns: ColumnDef<DeliveryLog>[] = [
  {
    accessorKey: "requestId",
    header: "Request",
    cell: ({ row }) => (
      <div className="group/row flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{row.original.requestId}</span>
        <CopyButton value={row.original.requestId} label="Copy request id" />
      </div>
    ),
  },
  {
    accessorKey: "provider",
    header: "Provider",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] text-muted-foreground">
        {row.original.provider}
      </span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StateBadge state={row.original.status} />,
  },
  {
    accessorKey: "latencyMs",
    header: "Latency",
    cell: ({ row }) =>
      row.original.latencyMs > 0 ? (
        <span className="font-mono text-[13px] tabular-nums">
          {row.original.latencyMs}
          <span className="text-muted-foreground"> ms</span>
        </span>
      ) : (
        <span className="text-muted-foreground">-</span>
      ),
  },
  {
    accessorKey: "error",
    header: "Detail",
    cell: ({ row }) =>
      row.original.error ? (
        <Tooltip>
          <TooltipTrigger
            render={
              <span className="block max-w-[220px] truncate font-mono text-[13px] text-[var(--state-failed)]" />
            }
          >
            {row.original.error}
          </TooltipTrigger>
          <TooltipContent>{row.original.error}</TooltipContent>
        </Tooltip>
      ) : (
        <span className="text-muted-foreground">-</span>
      ),
  },
];

function isFresh(createdAt: string): string | undefined {
  const age = Date.now() - new Date(createdAt).getTime();
  return age >= 0 && age < 20_000 ? "row-fresh" : undefined;
}

export function LogsView() {
  const { data, isLoading } = useLogs();

  if (isLoading || !data) return <Skeleton className="h-96 rounded-xl" />;
  if (data.length === 0) {
    return (
      <EmptyState
        icon={Truck}
        title="No deliveries yet"
        description="Send an OTP from the Playground to see it here."
      />
    );
  }

  return (
    <DataTable
      columns={columns}
      data={data}
      pageSize={12}
      rowKey={(l) => l.requestId}
      rowClassName={(l) => isFresh(l.createdAt)}
    />
  );
}
