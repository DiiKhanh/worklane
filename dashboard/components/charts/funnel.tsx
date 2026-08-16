"use client";

import { motion, useReducedMotion } from "motion/react";

type Funnel = { requested: number; sent: number; verified: number };

const STEPS = [
  { key: "requested", label: "Requested", color: "var(--state-requested)" },
  { key: "sent", label: "Delivered", color: "var(--state-sent)" },
  { key: "verified", label: "Verified", color: "var(--state-verified)" },
] as const;

export function FunnelChart({ data }: { data: Funnel }) {
  const reduce = useReducedMotion();
  const max = Math.max(data.requested, 1);

  return (
    <div className="space-y-4">
      {STEPS.map((step, i) => {
        const value = data[step.key];
        const pct = (value / max) * 100;
        const prev = i === 0 ? value : data[STEPS[i - 1].key];
        const conversion = prev ? Math.round((value / prev) * 100) : 0;
        return (
          <div key={step.key} className="space-y-1.5">
            <div className="flex items-baseline justify-between text-sm">
              <span className="flex items-center gap-2">
                <span
                  className="size-2 rounded-full"
                  style={{ background: step.color }}
                />
                <span className="text-muted-foreground">{step.label}</span>
              </span>
              <span className="flex items-baseline gap-2">
                <span className="font-mono tabular-nums font-medium">
                  {value.toLocaleString("en-US")}
                </span>
                {i > 0 && (
                  <span className="font-mono text-xs text-muted-foreground tabular-nums">
                    {conversion}%
                  </span>
                )}
              </span>
            </div>
            <div className="h-2.5 overflow-hidden rounded-full bg-muted">
              <motion.div
                className="h-full rounded-full"
                style={{ background: step.color }}
                initial={reduce ? false : { width: 0 }}
                animate={{ width: `${pct}%` }}
                transition={{
                  duration: 0.6,
                  delay: reduce ? 0 : i * 0.08,
                  ease: [0.23, 1, 0.32, 1],
                }}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}
