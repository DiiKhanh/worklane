"use client";

import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from "recharts";

const COLOR = "var(--state-verified)";
// Day labels for a 14-point series (newest last), matching the kit.
const DAY_LABELS = ["", "", "", "10d", "", "", "7d", "", "", "4d", "", "", "1d", "now"];

/**
 * Clicks-over-time area chart for a short link. Uses Recharts (already a repo
 * dependency) with the same visual language as the Overview SendsArea:
 * gradient fill, dashed horizontal gridlines, muted 11px ticks.
 */
export function ClicksArea({ series }: { series: number[] }) {
  const data = series.map((v, i) => ({ i, v, label: DAY_LABELS[i] ?? "" }));
  return (
    <ResponsiveContainer width="100%" height={180}>
      <AreaChart data={data} margin={{ top: 8, right: 8, left: -16, bottom: 0 }}>
        <defs>
          <linearGradient id="grad-clicks" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={COLOR} stopOpacity={0.35} />
            <stop offset="100%" stopColor={COLOR} stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <CartesianGrid
          strokeDasharray="3 3"
          vertical={false}
          stroke="var(--border)"
        />
        <XAxis
          dataKey="label"
          tickLine={false}
          axisLine={false}
          interval={0}
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
        />
        <YAxis
          allowDecimals={false}
          tickLine={false}
          axisLine={false}
          width={36}
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
        />
        <Area
          type="monotone"
          dataKey="v"
          stroke={COLOR}
          strokeWidth={1.5}
          fill="url(#grad-clicks)"
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}
