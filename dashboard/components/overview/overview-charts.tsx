"use client";

import { BarChart3 } from "lucide-react";
import { useOverview } from "@/lib/queries/use-overview";
import { SendsArea } from "@/components/charts/sends-area";
import { FunnelChart } from "@/components/charts/funnel";
import { Panel } from "@/components/common/panel";
import { Skeleton } from "@/components/ui/skeleton";

// Shown in place of a chart when useOverview errors (live API has no /v1/stats yet).
// className carries the height so the panel doesn't collapse / shift vs. the skeleton.
function Unavailable({ className }: { className?: string }) {
  return (
    <div
      className={`flex flex-col items-center justify-center gap-2 text-center ${className ?? ""}`}
    >
      <BarChart3 className="size-5 text-muted-foreground" />
      <p className="text-sm text-muted-foreground">
        Not available from the live API yet
      </p>
    </div>
  );
}

export function OverviewCharts() {
  const { data, isLoading, isError } = useOverview();

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <Panel
        title="Verification volume"
        description="Last 24 hours, by outcome"
        className="lg:col-span-2"
      >
        {isError ? (
          <Unavailable className="h-[260px]" />
        ) : isLoading || !data ? (
          <Skeleton className="h-[260px] rounded-lg" />
        ) : (
          <SendsArea data={data.series} />
        )}
      </Panel>

      <Panel title="Conversion funnel" description="Requested to verified">
        {isError ? (
          <Unavailable className="h-[144px]" />
        ) : isLoading || !data ? (
          <div className="space-y-4 py-2">
            <Skeleton className="h-8 rounded-md" />
            <Skeleton className="h-8 rounded-md" />
            <Skeleton className="h-8 rounded-md" />
          </div>
        ) : (
          <div className="pt-2">
            <FunnelChart data={data.funnel} />
          </div>
        )}
      </Panel>
    </div>
  );
}
