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
// Label every third day back from today so the newest bucket is always named.
const LABEL_EVERY = 3;

/** "2026-11-28" -> "Nov 28", read as a UTC calendar day. */
export function dayLabel(date: string): string {
  return new Date(`${date}T00:00:00Z`).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  });
}

/**
 * Daily recorded clicks for a short link over the 14 UTC days ending today.
 * Same visual language as the Overview SendsArea: gradient fill, dashed
 * horizontal gridlines, muted 11px ticks.
 */
export function ClicksArea({ series }: { series: { date: string; clicks: number }[] }) {
  const last = series.length - 1;
  const data = series.map((d, i) => ({
    v: d.clicks,
    label: i === last ? "Today" : (last - i) % LABEL_EVERY === 0 ? dayLabel(d.date) : "",
  }));
  return (
    <ResponsiveContainer width="100%" height={180}>
      <AreaChart data={data} margin={{ top: 8, right: 20, left: -16, bottom: 0 }}>
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
