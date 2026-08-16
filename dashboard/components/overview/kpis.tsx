"use client";

import { motion } from "motion/react";
import { Send, ShieldCheck, TriangleAlert, Timer } from "lucide-react";
import { useOverview } from "@/lib/queries/use-overview";
import { StatCard } from "@/components/common/stat-card";
import { CountUp } from "@/components/common/count-up";
import { Skeleton } from "@/components/ui/skeleton";

export function Kpis() {
  const { data, isLoading } = useOverview();

  if (isLoading || !data) {
    return (
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-[104px] rounded-xl" />
        ))}
      </div>
    );
  }

  const cards = [
    {
      label: "Sent today",
      icon: Send,
      accent: "var(--state-sent)",
      value: <CountUp value={data.sentToday} />,
      hint: "delivery attempts",
    },
    {
      label: "Verify rate",
      icon: ShieldCheck,
      accent: "var(--state-verified)",
      value: (
        <CountUp
          value={data.verifyRate}
          format={(v) => `${Math.round(v * 100)}%`}
        />
      ),
      hint: "of delivered codes",
    },
    {
      label: "Failed",
      icon: TriangleAlert,
      accent: "var(--state-failed)",
      value: <CountUp value={data.failed} />,
      hint: "routed to DLQ",
    },
    {
      label: "p50 latency",
      icon: Timer,
      accent: "var(--primary)",
      value: (
        <>
          <CountUp value={data.p50LatencyMs} />
          <span className="ml-1 text-base font-normal text-muted-foreground">
            ms
          </span>
        </>
      ),
      hint: "provider send time",
    },
  ];

  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      {cards.map((c, i) => (
        <motion.div
          key={c.label}
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35, delay: i * 0.05, ease: [0.23, 1, 0.32, 1] }}
        >
          <StatCard label={c.label} icon={c.icon} accent={c.accent} hint={c.hint}>
            {c.value}
          </StatCard>
        </motion.div>
      ))}
    </div>
  );
}
