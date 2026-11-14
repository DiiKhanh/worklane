"use client";

import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { LoaderCircle, ShieldCheck } from "lucide-react";
import { verifySchema, type VerifyValues, type Channel } from "@/lib/schemas";
import type { VerifyOutcome } from "@/lib/api/types";
import { useVerify } from "@/lib/queries/use-otp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PhoneInput } from "@/components/common/phone-input";

const OUTCOME: Record<VerifyOutcome, { label: string; color: string }> = {
  verified: { label: "Verified", color: "var(--state-verified)" },
  mismatch: { label: "Code mismatch", color: "var(--state-failed)" },
  expired: { label: "Code expired or unknown", color: "var(--state-expired)" },
  locked: { label: "Too many attempts", color: "var(--state-failed)" },
};

export function VerifyForm({
  channel,
  initial,
}: {
  channel: Channel;
  initial?: { recipient: string; code: string };
}) {
  const verify = useVerify();
  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<VerifyValues>({
    resolver: zodResolver(verifySchema(channel)),
    defaultValues: {
      recipient: initial?.recipient ?? "",
      code: initial?.code ?? "",
    },
  });

  const onSubmit = handleSubmit((values) =>
    verify.mutate({ recipient: values.recipient, code: values.code, channel }),
  );
  const outcome = verify.isSuccess ? OUTCOME[verify.data.status] : null;

  return (
    <form onSubmit={onSubmit} aria-busy={verify.isPending} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="verify-recipient">
          {channel === "sms" ? "Recipient phone" : "Recipient email"}
        </Label>
        {channel === "sms" ? (
          <Controller
            control={control}
            name="recipient"
            render={({ field }) => (
              <PhoneInput
                id="verify-recipient"
                value={field.value ?? ""}
                onChange={field.onChange}
              />
            )}
          />
        ) : (
          <Input
            id="verify-recipient"
            placeholder="user@example.com"
            autoComplete="off"
            aria-invalid={!!errors.recipient}
            {...register("recipient")}
          />
        )}
        {errors.recipient && (
          <p className="text-xs text-destructive">{errors.recipient.message}</p>
        )}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="verify-code">6-digit code</Label>
        <Input
          id="verify-code"
          autoFocus={!!initial}
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
        {verify.isPending ? (
          <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" />
        ) : (
          <ShieldCheck className="size-4" />
        )}
        {verify.isPending ? "Verifying..." : "Verify code"}
      </Button>

      {verify.isError && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
        >
          Could not verify the code. Check the connection and try again.
        </p>
      )}

      {outcome && (
        <div
          role="status"
          aria-label="Verification result"
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
        </div>
      )}
    </form>
  );
}
