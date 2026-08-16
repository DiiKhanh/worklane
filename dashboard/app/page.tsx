import { Kpis } from "@/components/overview/kpis";
import { ActivityFeed } from "@/components/overview/activity-feed";
import { Panel } from "@/components/common/panel";
import { OverviewCharts } from "@/components/overview/overview-charts";

export default function OverviewPage() {
  const isMock = process.env.NEXT_PUBLIC_DATA_SOURCE !== "live";
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold tracking-tight">Overview</h2>
        <p className="text-sm text-muted-foreground">
          Verification traffic across your tenant, updating live.
        </p>
      </div>

      <Kpis />

      <OverviewCharts />

      <Panel title="Recent activity" description="Latest OTP requests">
        <ActivityFeed />
      </Panel>

      {isMock && (
        <p className="text-center text-xs text-muted-foreground">
          Aggregate metrics are sample data - the live API exposes raw events;
          a stats endpoint is planned.
        </p>
      )}
    </div>
  );
}
