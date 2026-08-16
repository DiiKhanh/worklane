"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { KeyRound } from "lucide-react";
import type { ApiKey } from "@/lib/api/types";
import { useApiKeys } from "@/lib/queries/use-api-keys";
import { DataTable } from "@/components/common/data-table";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { EmptyState } from "@/components/common/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { timeAgo } from "@/lib/format";

const columns: ColumnDef<ApiKey>[] = [
  {
    accessorKey: "id",
    header: "Key",
    cell: ({ row }) => (
      <div className="group/row flex items-center gap-1.5">
        <span className="font-mono text-[13px]">{row.original.id}</span>
        <CopyButton value={row.original.id} label="Copy key id" />
      </div>
    ),
  },
  {
    accessorKey: "tenantId",
    header: "Tenant",
    cell: ({ row }) => (
      <span className="font-mono text-[13px] text-muted-foreground">
        {row.original.tenantId}
      </span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StateBadge state={row.original.status} />,
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

export function ApiKeysTable() {
  const { data, isLoading } = useApiKeys();

  if (isLoading || !data) return <Skeleton className="h-64 rounded-xl" />;
  if (data.length === 0) {
    return (
      <EmptyState
        icon={KeyRound}
        title="No API keys yet"
        description="Keys are provisioned by the seed CLI for now."
      />
    );
  }

  return <DataTable columns={columns} data={data} pageSize={10} />;
}
