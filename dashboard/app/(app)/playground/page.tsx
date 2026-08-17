import { SectionHeading } from "@/components/common/section-heading";
import { Playground } from "@/components/playground/playground";

export default function PlaygroundPage() {
  return (
    <div>
      <SectionHeading
        title="Playground"
        description="Exercise the full send-then-verify loop without writing any code."
      />
      <Playground />
    </div>
  );
}
