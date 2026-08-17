import { Info } from "lucide-react";

/**
 * The dashed "this is a proposal" banner shown on roadmap screens (Templates,
 * Links, Campaigns). Copy is fixed and matches the design-system UI kit.
 */
export function RoadmapNote() {
  return (
    <div className="mb-4 flex items-center gap-2 rounded-xl border border-dashed border-border px-3 py-2 text-xs text-muted-foreground">
      <Info className="size-3.5 shrink-0" />
      Roadmap screen. Not in the codebase yet - laid out only from shipped
      worklane patterns.
    </div>
  );
}
