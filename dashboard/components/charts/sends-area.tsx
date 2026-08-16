"use client";

import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { OverviewSeriesPoint } from "@/lib/api/types";

const SERIES = [
  { key: "verified", label: "Verified", color: "var(--state-verified)" },
  { key: "sent", label: "Sent", color: "var(--state-sent)" },
  { key: "failed", label: "Failed", color: "var(--state-failed)" },
  { key: "requested", label: "Pending", color: "var(--state-requested)" },
] as const;

function hour(t: string): string {
  return new Date(t).toLocaleTimeString("en-US", { hour: "numeric" });
}

type TooltipEntry = { name: string; value: number; color: string };

function ChartTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: TooltipEntry[];
  label?: string;
}) {
  if (!active || !payload?.length) return null;
  const total = payload.reduce((s, p) => s + (p.value ?? 0), 0);
  return (
    <div className="rounded-lg border border-border bg-popover/95 px-3 py-2 text-xs shadow-md backdrop-blur">
      <p className="mb-1.5 font-medium text-foreground">
        {label ? new Date(label).toLocaleString("en-US", {
          hour: "numeric",
          weekday: "short",
        }) : ""}
      </p>
      <div className="space-y-1">
        {payload.map((p) => (
          <div key={p.name} className="flex items-center gap-2">
            <span
              className="size-2 rounded-full"
              style={{ background: p.color }}
            />
            <span className="text-muted-foreground">{p.name}</span>
            <span className="ml-auto font-mono tabular-nums text-foreground">
              {p.value}
            </span>
          </div>
        ))}
        <div className="mt-1 flex items-center gap-2 border-t border-border pt-1">
          <span className="text-muted-foreground">Total</span>
          <span className="ml-auto font-mono tabular-nums text-foreground">
            {total}
          </span>
        </div>
      </div>
    </div>
  );
}

export function SendsArea({ data }: { data: OverviewSeriesPoint[] }) {
  return (
    <ResponsiveContainer width="100%" height={260}>
      <AreaChart data={data} margin={{ top: 8, right: 8, left: -16, bottom: 0 }}>
        <defs>
          {SERIES.map((s) => (
            <linearGradient
              key={s.key}
              id={`grad-${s.key}`}
              x1="0"
              y1="0"
              x2="0"
              y2="1"
            >
              <stop offset="0%" stopColor={s.color} stopOpacity={0.35} />
              <stop offset="100%" stopColor={s.color} stopOpacity={0.02} />
            </linearGradient>
          ))}
        </defs>
        <CartesianGrid
          strokeDasharray="3 3"
          vertical={false}
          stroke="var(--border)"
        />
        <XAxis
          dataKey="t"
          tickFormatter={hour}
          tickLine={false}
          axisLine={false}
          minTickGap={28}
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
        />
        <YAxis
          allowDecimals={false}
          tickLine={false}
          axisLine={false}
          width={36}
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
        />
        <Tooltip
          content={<ChartTooltip />}
          cursor={{ stroke: "var(--border)", strokeWidth: 1 }}
        />
        {SERIES.map((s) => (
          <Area
            key={s.key}
            type="monotone"
            dataKey={s.key}
            name={s.label}
            stackId="1"
            stroke={s.color}
            strokeWidth={1.5}
            fill={`url(#grad-${s.key})`}
            isAnimationActive={false}
          />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  );
}
