"use client";

import { ArrowLeft } from "lucide-react";
import type { DeliveryLog, OtpRequest } from "@/lib/api/types";
import { lifecycleStages, type StageState } from "@/lib/requests/lifecycle";
import { Button } from "@/components/ui/button";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { timeAgo } from "@/lib/format";

const STAGE_COLOR: Record<StageState, string> = {
  done: "var(--state-verified)",
  failed: "var(--state-failed)",
  pending: "var(--muted-foreground)",
};

function Timeline({
  request,
  log,
}: {
  request: OtpRequest;
  log?: DeliveryLog;
}) {
  const stages = lifecycleStages(request.state, log);
  return (
    <div className="grid">
      {stages.map((stage, i) => {
        const color = STAGE_COLOR[stage.state];
        const filled = stage.state !== "pending";
        const last = i === stages.length - 1;
        const Icon = stage.icon;
        return (
          <div key={stage.key} className="grid grid-cols-[28px_1fr] gap-3">
            <div className="flex flex-col items-center">
              <span
                className="flex size-7 items-center justify-center rounded-full border"
                style={{
                  color,
                  background: filled
                    ? `color-mix(in oklch, ${color} 14%, transparent)`
                    : "var(--muted)",
                  borderColor: filled
                    ? `color-mix(in oklch, ${color} 35%, transparent)`
                    : "var(--border)",
                }}
              >
                <Icon className="size-3.5" />
              </span>
              {!last && (
                <span
                  className="w-0.5 flex-1"
                  style={{
                    minHeight: 22,
                    background:
                      stages[i + 1].state === "done"
                        ? "color-mix(in oklch, var(--state-verified) 35%, transparent)"
                        : "var(--border)",
                  }}
                />
              )}
            </div>
            <div className={last ? "" : "pb-4"}>
              <div
                className="text-sm font-medium"
                style={{
                  color: filled ? "var(--foreground)" : "var(--muted-foreground)",
                }}
              >
                {stage.label}
              </div>
              <div className="text-xs text-muted-foreground">{stage.detail}</div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="group/row flex items-center justify-between gap-3 border-t border-border/70 py-2.5 first:border-t-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="inline-flex items-center gap-1.5">{children}</span>
    </div>
  );
}

export function RequestDetail({
  request,
  log,
  onBack,
}: {
  request: OtpRequest;
  log?: DeliveryLog;
  onBack: () => void;
}) {
  return (
    <div>
      <Button
        variant="ghost"
        size="sm"
        onClick={onBack}
        className="-ml-2.5 mb-3"
      >
        <ArrowLeft className="size-4" />
        OTP requests
      </Button>
      <SectionHeading
        title="Request detail"
        description={request.id}
        action={<StateBadge state={request.state} />}
      />

      <div className="grid gap-4 md:grid-cols-2">
        <Panel title="Lifecycle" description="Events emitted for this code">
          <Timeline request={request} log={log} />
        </Panel>
        <Panel title="Attributes">
          <div className="grid">
            <Field label="Request id">
              <span className="font-mono text-[13px]">{request.id}</span>
              <CopyButton value={request.id} label="Copy request id" />
            </Field>
            <Field label="Recipient">
              <span className="font-mono text-[13px] text-muted-foreground">
                {request.recipient}
              </span>
            </Field>
            <Field label="Channel">
              <span className="capitalize">{request.channel}</span>
            </Field>
            <Field label="Provider">
              <span className="font-mono text-[13px] text-muted-foreground">
                {log?.provider ?? "-"}
              </span>
            </Field>
            {log && log.latencyMs > 0 && (
              <Field label="Latency">
                <span className="font-mono text-[13px] tabular-nums">
                  {log.latencyMs}
                  <span className="text-muted-foreground"> ms</span>
                </span>
              </Field>
            )}
            <Field label="Created">
              <span className="text-sm text-muted-foreground tabular-nums">
                {timeAgo(request.createdAt) || "-"}
              </span>
            </Field>
          </div>
        </Panel>
      </div>

      {log?.error && (
        <Panel title="Provider response" className="mt-4">
          <div
            className="rounded-xl border p-3 font-mono text-[13px]"
            style={{
              color: "var(--state-failed)",
              borderColor: "color-mix(in oklch, var(--state-failed) 30%, transparent)",
              background: "color-mix(in oklch, var(--state-failed) 10%, transparent)",
            }}
          >
            {log.error}
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            Failed sends are retried with exponential backoff, then routed to the
            DLQ.
          </p>
        </Panel>
      )}
    </div>
  );
}
