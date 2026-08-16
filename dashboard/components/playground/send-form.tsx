"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, motion } from "motion/react";
import { Send, ArrowRight } from "lucide-react";
import { sendSchema, type SendValues } from "@/lib/schemas";
import { useSend } from "@/lib/queries/use-otp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CopyButton } from "@/components/common/copy-button";

export function SendForm({
  onSent,
}: {
  onSent?: (recipient: string, code: string) => void;
}) {
  const send = useSend();
  const {
    register,
    handleSubmit,
    getValues,
    formState: { errors },
  } = useForm<SendValues>({
    resolver: zodResolver(sendSchema),
    defaultValues: { recipient: "" },
  });

  const onSubmit = handleSubmit((values) => send.mutate(values.recipient));
  const result = send.data;

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="send-recipient">Recipient email</Label>
        <Input
          id="send-recipient"
          placeholder="user@example.com"
          autoComplete="off"
          aria-invalid={!!errors.recipient}
          {...register("recipient")}
        />
        {errors.recipient && (
          <p className="text-xs text-destructive">{errors.recipient.message}</p>
        )}
      </div>

      <Button type="submit" disabled={send.isPending} className="w-full">
        <Send className="size-4" />
        {send.isPending ? "Sending..." : "Send code"}
      </Button>

      <AnimatePresence initial={false}>
        {result && (
          <motion.div
            initial={{ opacity: 0, y: 6, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={{ duration: 0.25, ease: [0.23, 1, 0.32, 1] }}
            className="space-y-3 rounded-lg border border-border bg-muted/40 p-4"
          >
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Request</span>
              <span className="group/row flex items-center gap-1.5">
                <span className="font-mono text-[13px]">{result.requestId}</span>
                <CopyButton value={result.requestId} />
              </span>
            </div>
            {result.devCode && (
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">
                  Code (dev only)
                </span>
                <span className="font-mono text-lg font-semibold tracking-[0.3em] text-foreground">
                  {result.devCode}
                </span>
              </div>
            )}
            {result.devCode && onSent && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-full"
                onClick={() => onSent(getValues("recipient"), result.devCode!)}
              >
                Use this code to verify
                <ArrowRight className="size-4" />
              </Button>
            )}
          </motion.div>
        )}
      </AnimatePresence>
    </form>
  );
}
