"use client";

import { useEffect, useState } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";

const COPIED_MS = 1500;

/** A labelled copy button for a short URL, with visible success and failure feedback. */
export function CopyLinkButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const id = setTimeout(() => setCopied(false), COPIED_MS);
    return () => clearTimeout(id);
  }, [copied]);

  async function onCopy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
    } catch {
      // The clipboard is unavailable outside secure contexts or when permission is denied.
      toast.error("Could not copy the link", { description: value });
    }
  }

  return (
    <Button size="sm" variant="outline" onClick={onCopy}>
      {copied ? (
        <Check className="size-3.5 text-[var(--state-verified)]" />
      ) : (
        <Copy className="size-3.5" />
      )}
      <span aria-live="polite">{copied ? "Copied" : "Copy link"}</span>
    </Button>
  );
}
