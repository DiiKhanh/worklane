import { Plus } from "lucide-react";
import { SectionHeading } from "@/components/common/section-heading";
import { ApiKeysTable } from "@/components/api-keys/api-keys-table";
import { Button } from "@/components/ui/button";

export default function ApiKeysPage() {
  return (
    <div>
      <SectionHeading
        title="API keys"
        description="Bearer credentials your services use to call the OTP API."
        action={
          <Button
            variant="outline"
            size="sm"
            disabled
            title="Self-serve key management is coming in Phase 2"
          >
            <Plus className="size-4" />
            New key
          </Button>
        }
      />
      <ApiKeysTable />
    </div>
  );
}
