import { cn } from "@/lib/utils";

/** A softly pulsing dot that signals a live, polling data stream. */
export function LiveDot({ className }: { className?: string }) {
  return (
    <span className={cn("relative flex size-2", className)}>
      <span
        className="absolute inline-flex size-full animate-ping rounded-full opacity-60 motion-reduce:hidden"
        style={{ background: "var(--state-verified)" }}
      />
      <span
        className="relative inline-flex size-2 rounded-full"
        style={{ background: "var(--state-verified)" }}
      />
    </span>
  );
}
