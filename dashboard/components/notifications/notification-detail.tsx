"use client";

import { ArrowLeft } from "lucide-react";
import { useNotification } from "@/lib/queries/use-notifications";
import { Button } from "@/components/ui/button";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { StateBadge } from "@/components/common/state-badge";
import { CopyButton } from "@/components/common/copy-button";
import { timeAgo } from "@/lib/format";

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="group/row flex flex-wrap items-start justify-between gap-x-3 gap-y-1 border-t border-border/70 py-2.5 first:border-t-0">
      <span className="shrink-0 text-sm text-muted-foreground">{label}</span>
      <span className="inline-flex min-w-0 max-w-full flex-1 items-center justify-end gap-1.5 text-right break-all">
        {children}
      </span>
    </div>
  );
}

function Mono({ children }: { children: React.ReactNode }) {
  return (
    <span className="min-w-0 font-mono text-[13px] text-muted-foreground break-all">
      {children}
    </span>
  );
}

export function NotificationDetail({
  id,
  onBack,
}: {
  id: string;
  onBack: () => void;
}) {
  const { data: n, isLoading, error } = useNotification(id);

  const back = (
    <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
      <ArrowLeft className="size-4" />
      Notifications
    </Button>
  );

  if (isLoading || !n) {
    return (
      <div>
        {back}
        {error ? (
          <p role="alert" className="py-10 text-center text-sm text-destructive">
            Could not load this notification. {error.message}
          </p>
        ) : (
          <div className="py-10 text-center text-sm text-muted-foreground">
            Loading notification…
          </div>
        )}
      </div>
    );
  }

  return (
    <div>
      {back}
      <SectionHeading
        title="Notification detail"
        description={n.id}
        action={<StateBadge state={n.state} />}
      />

      <div className="grid gap-4 xl:grid-cols-2">
        <Panel title="Attributes">
          <div className="grid">
            <Field label="Notification id">
              <Mono>{n.id}</Mono>
              <CopyButton
                value={n.id}
                label="Copy notification id"
                className="shrink-0"
              />
            </Field>
            <Field label="Recipient">
              <Mono>{n.recipient}</Mono>
            </Field>
            <Field label="Channel">
              <span className="capitalize">{n.channel}</span>
            </Field>
            <Field label="Kind">
              <span className="capitalize">{n.kind}</span>
            </Field>
            <Field label="Template">
              <Mono>{n.templateId || "-"}</Mono>
            </Field>
            <Field label="Provider">
              <Mono>{n.provider || "-"}</Mono>
            </Field>
            {n.providerMsgId && (
              <Field label="Provider message id">
                <Mono>{n.providerMsgId}</Mono>
              </Field>
            )}
            {n.latencyMs > 0 && (
              <Field label="Latency">
                <span className="font-mono text-[13px] tabular-nums">
                  {n.latencyMs}
                  <span className="text-muted-foreground"> ms</span>
                </span>
              </Field>
            )}
            <Field label="Created">
              <span className="text-sm text-muted-foreground tabular-nums">
                {timeAgo(n.createdAt) || "-"}
              </span>
            </Field>
          </div>
        </Panel>

        <Panel title="Engagement" description="Events recorded after delivery">
          {n.events.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {n.state === "suppressed"
                ? "Suppressed by the recipient's preferences, so nothing was sent."
                : "No engagement events recorded."}
            </p>
          ) : (
            <ul className="grid">
              {n.events.map((e, i) => (
                <li
                  key={`${e.type}-${e.ts}-${i}`}
                  className="flex items-center justify-between gap-3 border-t border-border/70 py-2.5 first:border-t-0"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="text-sm font-medium capitalize">{e.type}</span>
                    {e.meta && <Mono>{e.meta}</Mono>}
                  </span>
                  <span className="shrink-0 text-sm text-muted-foreground tabular-nums">
                    {timeAgo(e.ts) || "-"}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>

      {n.error && (
        <Panel title="Failure reason" className="mt-4">
          <div
            className="rounded-xl border p-3 font-mono text-[13px] break-words"
            style={{
              color: "var(--state-failed)",
              borderColor: "color-mix(in oklch, var(--state-failed) 30%, transparent)",
              background: "color-mix(in oklch, var(--state-failed) 10%, transparent)",
            }}
          >
            {n.error}
          </div>
        </Panel>
      )}
    </div>
  );
}
