"use client";

import { useState } from "react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Send, ArrowRight, LoaderCircle } from "lucide-react";
import { sendSchema, type SendValues, type Channel } from "@/lib/schemas";
import { useSend } from "@/lib/queries/use-otp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CopyButton } from "@/components/common/copy-button";
import { PhoneInput } from "@/components/common/phone-input";
import { SimpleSelect } from "@/components/common/simple-select";

export function SendForm({
  channel,
  onSent,
}: {
  channel: Channel;
  onSent?: (recipient: string, code: string) => void;
}) {
  const send = useSend();
  const [locale, setLocale] = useState("en");
  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<SendValues>({
    resolver: zodResolver(sendSchema(channel)),
    defaultValues: { recipient: "" },
  });

  const onSubmit = handleSubmit((values) =>
    send.mutate({ recipient: values.recipient, channel, locale }),
  );
  const result = send.isSuccess ? send.data : undefined;
  const sentRecipient = send.variables?.recipient;

  return (
    <form onSubmit={onSubmit} aria-busy={send.isPending} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="send-recipient">
          {channel === "sms" ? "Recipient phone" : "Recipient email"}
        </Label>
        {channel === "sms" ? (
          <Controller
            control={control}
            name="recipient"
            render={({ field }) => (
              <PhoneInput
                id="send-recipient"
                value={field.value ?? ""}
                onChange={field.onChange}
              />
            )}
          />
        ) : (
          <Input
            id="send-recipient"
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
        <Label htmlFor="send-locale">Locale</Label>
        <SimpleSelect
          id="send-locale"
          value={locale}
          onValueChange={setLocale}
          options={[
            { value: "en", label: "en - English" },
            { value: "vi", label: "vi - Tiếng Việt" },
          ]}
        />
      </div>

      <Button type="submit" disabled={send.isPending} className="w-full">
        {send.isPending ? (
          <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" />
        ) : (
          <Send className="size-4" />
        )}
        {send.isPending ? "Sending..." : "Send code"}
      </Button>

      {send.isError && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
        >
          Could not send the code. Check the connection and try again.
        </p>
      )}

      {result && (
        <div
          role="status"
          aria-label="Send result"
          className="space-y-3 rounded-lg border border-border bg-muted/40 p-4"
        >
          <div className="space-y-1">
            <p className="text-sm font-medium">Code issued</p>
            <p className="break-all text-xs text-muted-foreground">
              {sentRecipient}
            </p>
            <p className="text-xs text-muted-foreground">
              Delivery status is available in Delivery logs.
            </p>
          </div>
          <div className="flex flex-col gap-2 text-sm sm:flex-row sm:items-center sm:justify-between">
            <span className="text-muted-foreground">Request</span>
            <span className="group/row flex items-center gap-1.5">
              <span className="break-all font-mono text-[13px]">
                {result.requestId}
              </span>
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
          {result.devCode && sentRecipient && onSent && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="w-full"
              onClick={() => onSent(sentRecipient, result.devCode!)}
            >
              Use this code to verify
              <ArrowRight className="size-4" />
            </Button>
          )}
        </div>
      )}
    </form>
  );
}
