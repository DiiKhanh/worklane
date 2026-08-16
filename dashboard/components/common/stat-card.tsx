import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function StatCard({
  label,
  icon: Icon,
  children,
  hint,
  accent = "var(--primary)",
  className,
}: {
  label: string;
  icon: LucideIcon;
  children: ReactNode;
  hint?: ReactNode;
  accent?: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "group/stat relative overflow-hidden rounded-xl border border-border bg-card p-4 transition-colors hover:border-foreground/15",
        className,
      )}
    >
      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">{label}</span>
        <span
          className="flex size-7 items-center justify-center rounded-md"
          style={{
            color: accent,
            background: `color-mix(in oklch, ${accent} 12%, transparent)`,
          }}
        >
          <Icon className="size-4" />
        </span>
      </div>
      <div className="mt-3 text-2xl font-semibold tabular-nums tracking-tight">
        {children}
      </div>
      {hint && (
        <div className="mt-1 text-xs text-muted-foreground">{hint}</div>
      )}
    </div>
  );
}
