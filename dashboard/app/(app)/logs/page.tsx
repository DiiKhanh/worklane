import { SectionHeading } from "@/components/common/section-heading";
import { LogsView } from "@/components/logs/logs-view";
import { LiveDot } from "@/components/common/live-dot";

export default function LogsPage() {
  return (
    <div>
      <SectionHeading
        title="Delivery logs"
        description="Per-attempt provider results from the dispatcher."
        action={
          <span className="inline-flex items-center gap-2 rounded-full border border-border bg-card/60 px-3 py-1 text-xs text-muted-foreground">
            <LiveDot />
            Live · every 4s
          </span>
        }
      />
      <LogsView />
    </div>
  );
}
