"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, motion } from "motion/react";
import { ShieldCheck } from "lucide-react";
import { verifySchema, type VerifyValues } from "@/lib/schemas";
import type { VerifyOutcome } from "@/lib/api/types";
import { useVerify } from "@/lib/queries/use-otp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const OUTCOME: Record<VerifyOutcome, { label: string; color: string }> = {
  verified: { label: "Verified", color: "var(--state-verified)" },
  mismatch: { label: "Code mismatch", color: "var(--state-failed)" },
  expired: { label: "Code expired or unknown", color: "var(--state-expired)" },
  locked: { label: "Too many attempts", color: "var(--state-failed)" },
};

export function VerifyForm({
  initial,
}: {
  initial?: { recipient: string; code: string };
}) {
  const verify = useVerify();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<VerifyValues>({
    resolver: zodResolver(verifySchema),
    defaultValues: {
      recipient: initial?.recipient ?? "",
      code: initial?.code ?? "",
    },
  });

  const onSubmit = handleSubmit((values) =>
    verify.mutate({ recipient: values.recipient, code: values.code }),
  );
  const outcome = verify.data ? OUTCOME[verify.data.status] : null;

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="verify-recipient">Recipient email</Label>
        <Input
          id="verify-recipient"
          placeholder="user@example.com"
          autoComplete="off"
          aria-invalid={!!errors.recipient}
          {...register("recipient")}
        />
        {errors.recipient && (
          <p className="text-xs text-destructive">{errors.recipient.message}</p>
        )}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="verify-code">6-digit code</Label>
        <Input
          id="verify-code"
          inputMode="numeric"
          maxLength={6}
          placeholder="000000"
          autoComplete="off"
          aria-invalid={!!errors.code}
          className="font-mono tracking-[0.3em]"
          {...register("code")}
        />
        {errors.code && (
          <p className="text-xs text-destructive">{errors.code.message}</p>
        )}
      </div>

      <Button type="submit" disabled={verify.isPending} className="w-full">
        <ShieldCheck className="size-4" />
        {verify.isPending ? "Verifying..." : "Verify code"}
      </Button>

      <AnimatePresence initial={false}>
        {outcome && (
          <motion.div
            initial={{ opacity: 0, y: 6, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={{ duration: 0.25, ease: [0.23, 1, 0.32, 1] }}
            className="flex items-center gap-2.5 rounded-lg border p-3 text-sm"
            style={{
              borderColor: `color-mix(in oklch, ${outcome.color} 30%, transparent)`,
              background: `color-mix(in oklch, ${outcome.color} 10%, transparent)`,
              color: outcome.color,
            }}
          >
            <span
              className="size-2 rounded-full"
              style={{ background: outcome.color }}
            />
            <span className="font-medium">{outcome.label}</span>
          </motion.div>
        )}
      </AnimatePresence>
    </form>
  );
}
