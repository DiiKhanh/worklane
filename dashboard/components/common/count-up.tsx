"use client";

import { useEffect, useState } from "react";
import { animate, useReducedMotion } from "motion/react";

/**
 * Animates a number from 0 to `value` with a strong ease-out. Under reduced
 * motion it renders the final value immediately (comprehension over motion).
 */
export function CountUp({
  value,
  format,
  durationMs = 750,
  className,
}: {
  value: number;
  format?: (v: number) => string;
  durationMs?: number;
  className?: string;
}) {
  const reduce = useReducedMotion();
  const [display, setDisplay] = useState(value);

  useEffect(() => {
    if (reduce) {
      setDisplay(value);
      return;
    }
    setDisplay(0);
    const controls = animate(0, value, {
      duration: durationMs / 1000,
      ease: [0.23, 1, 0.32, 1],
      onUpdate: (v) => setDisplay(v),
    });
    return () => controls.stop();
  }, [value, reduce, durationMs]);

  const text = format
    ? format(display)
    : Math.round(display).toLocaleString("en-US");

  return (
    <span className={className} suppressHydrationWarning>
      {text}
    </span>
  );
}
