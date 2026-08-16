import { cn } from "@/lib/utils";

type BadgeState =
  | "requested"
  | "sent"
  | "verified"
  | "failed"
  | "expired"
  | "active"
  | "revoked";

const STATE_COLOR: Record<BadgeState, string> = {
  requested: "var(--state-requested)",
  sent: "var(--state-sent)",
  verified: "var(--state-verified)",
  failed: "var(--state-failed)",
  expired: "var(--state-expired)",
  active: "var(--state-verified)",
  revoked: "var(--state-expired)",
};

export function StateBadge({
  state,
  className,
}: {
  state: BadgeState;
  className?: string;
}) {
  const color = STATE_COLOR[state] ?? "var(--muted-foreground)";
  return (
    <span
      data-state={state}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium capitalize",
        className,
      )}
      style={{
        color,
        borderColor: `color-mix(in oklch, ${color} 30%, transparent)`,
        background: `color-mix(in oklch, ${color} 12%, transparent)`,
      }}
    >
      <span
        className="size-1.5 rounded-full"
        style={{ background: color }}
        aria-hidden
      />
      {state}
    </span>
  );
}
