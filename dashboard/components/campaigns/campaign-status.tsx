import type { CampaignStatusValue } from "@/lib/roadmap/campaigns";
import { Badge } from "@/components/ui/badge";
import { StateBadge } from "@/components/common/state-badge";

/** Maps a campaign's lifecycle to the shared state vocabulary. */
export function CampaignStatus({ status }: { status: CampaignStatusValue }) {
  if (status === "sent") return <StateBadge state="verified" />;
  if (status === "sending") return <StateBadge state="sent" />;
  if (status === "scheduled") return <Badge variant="outline">scheduled</Badge>;
  return <Badge variant="secondary">draft</Badge>;
}
