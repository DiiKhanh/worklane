"use client";

import { useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

export function CopyButton({
  value,
  className,
  label = "Copy",
}: {
  value: string;
  className?: string;
  label?: string;
}) {
  const [copied, setCopied] = useState(false);

  async function onCopy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard can be unavailable (insecure context); fail quietly.
    }
  }

  return (
    <button
      type="button"
      onClick={onCopy}
      aria-label={copied ? "Copied" : label}
      className={cn(
        "inline-flex size-6 items-center justify-center rounded-md text-muted-foreground opacity-0 transition-[color,opacity] hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring/60 group-hover/row:opacity-100 motion-reduce:opacity-100",
        className,
      )}
    >
      <AnimatePresence mode="wait" initial={false}>
        <motion.span
          key={copied ? "check" : "copy"}
          initial={{ scale: 0.6, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          exit={{ scale: 0.6, opacity: 0 }}
          transition={{ type: "spring", duration: 0.25, bounce: 0.35 }}
        >
          {copied ? (
            <Check className="size-3.5 text-[var(--state-verified)]" />
          ) : (
            <Copy className="size-3.5" />
          )}
        </motion.span>
      </AnimatePresence>
    </button>
  );
}
