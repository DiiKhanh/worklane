"use client";

import { useOverview } from "@/lib/queries/use-overview";
import { SendsArea } from "@/components/charts/sends-area";
import { FunnelChart } from "@/components/charts/funnel";
import { Panel } from "@/components/common/panel";
import { Skeleton } from "@/components/ui/skeleton";

export function OverviewCharts() {
  const { data, isLoading } = useOverview();

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <Panel
        title="Verification volume"
        description="Last 24 hours, by outcome"
        className="lg:col-span-2"
      >
        {isLoading || !data ? (
          <Skeleton className="h-[260px] rounded-lg" />
        ) : (
          <SendsArea data={data.series} />
        )}
      </Panel>

      <Panel title="Conversion funnel" description="Requested to verified">
        {isLoading || !data ? (
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
