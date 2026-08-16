import { SectionHeading } from "@/components/common/section-heading";
import { RequestsView } from "@/components/requests/requests-view";

export default function RequestsPage() {
  return (
    <div>
      <SectionHeading
        title="OTP requests"
        description="Every code issued for your tenant. Recipients are masked at rest."
      />
      <RequestsView />
    </div>
  );
}
