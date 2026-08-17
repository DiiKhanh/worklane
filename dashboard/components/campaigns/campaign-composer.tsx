"use client";

import { useState } from "react";
import { ArrowLeft, Clock, FlaskConical, Send, ShieldCheck, Upload } from "lucide-react";
import type { Campaign } from "@/lib/roadmap/campaigns";
import { AUDIENCES, CAMPAIGN_TEMPLATES } from "@/lib/roadmap/campaigns";
import { recipientScope, sendWindowMinutes } from "@/lib/roadmap/campaign-math";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { SectionHeading } from "@/components/common/section-heading";
import { Panel } from "@/components/common/panel";
import { SimpleSelect } from "@/components/common/simple-select";

const nf = (n: number) => n.toLocaleString("en-US");

/** A campaign the composer queues; the parent fills id/delivered/opened/clicked. */
export type QueuedCampaign = Omit<
  Campaign,
  "id" | "delivered" | "opened" | "clicked"
>;

function SummaryRow({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-baseline justify-between gap-3 border-t border-border/70 py-1.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-right text-sm">{children}</span>
    </div>
  );
}

export function CampaignComposer({
  onBack,
  onQueue,
}: {
  onBack: () => void;
  onQueue: (c: QueuedCampaign) => void;
}) {
  const [name, setName] = useState("September product news");
  const [kind, setKind] = useState<"marketing" | "otp">("marketing");
  const [audienceId, setAudienceId] = useState("aud_newsletter");
  const [csvName, setCsvName] = useState("");
  const [templateId, setTemplateId] = useState("tpl_product_news");
  const [fromName, setFromName] = useState("worklane");
  const [subject, setSubject] = useState(CAMPAIGN_TEMPLATES[2].subject);
  const [body, setBody] = useState(CAMPAIGN_TEMPLATES[2].body);
  const [timing, setTiming] = useState("now");
  const [when, setWhen] = useState("2026-08-19T09:00");
  const [rate, setRate] = useState(600);
  const [testTo, setTestTo] = useState("khanh@worklane.io");
  const [confirm, setConfirm] = useState(false);

  const audience = AUDIENCES.find((a) => a.id === audienceId)!;
  const tpl = CAMPAIGN_TEMPLATES.find((t) => t.id === templateId)!;
  const isCsv = audienceId === "aud_csv";
  const size = recipientScope({ isCsv, size: audience.size, csvLoaded: !!csvName });
  const minutes = sendWindowMinutes(size, rate);

  function pickTemplate(id: string) {
    const t = CAMPAIGN_TEMPLATES.find((x) => x.id === id)!;
    setTemplateId(id);
    setSubject(t.subject);
    setBody(t.body);
  }

  return (
    <div>
      <Button variant="ghost" size="sm" onClick={onBack} className="-ml-2.5 mb-3">
        <ArrowLeft className="size-4" />
        Campaigns
      </Button>
      <SectionHeading
        title="New campaign"
        description="Audience, message, then delivery. Nothing sends until you confirm."
      />

      <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="grid gap-4">
          <Panel title="Audience" description="Who receives this send">
            <div className="grid gap-4">
              <div className="grid gap-3 sm:grid-cols-[1fr_160px]">
                <div className="grid gap-1.5">
                  <Label htmlFor="cmp-name">Campaign name</Label>
                  <Input
                    id="cmp-name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="cmp-kind">Type</Label>
                  <SimpleSelect
                    id="cmp-kind"
                    value={kind}
                    onValueChange={(v) => setKind(v as "marketing" | "otp")}
                    options={[
                      { value: "marketing", label: "Marketing" },
                      { value: "otp", label: "OTP batch" },
                    ]}
                  />
                </div>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="cmp-aud">Recipient list</Label>
                <SimpleSelect
                  id="cmp-aud"
                  value={audienceId}
                  onValueChange={setAudienceId}
                  options={AUDIENCES.map((a) => ({
                    value: a.id,
                    label: a.size ? `${a.label} (${nf(a.size)})` : a.label,
                  }))}
                />
                <span className="text-xs text-muted-foreground">
                  {audience.note}
                </span>
              </div>
              {isCsv && (
                <div className="flex items-center gap-3 rounded-xl border border-dashed border-border px-3.5 py-3">
                  <Upload className="size-4 text-muted-foreground" />
                  <span className="flex-1 text-sm text-muted-foreground">
                    {csvName ? (
                      <span className="font-mono text-[13px]">
                        {csvName} · 218 rows · 0 invalid
                      </span>
                    ) : (
                      "CSV with an email or phone column, max 50,000 rows"
                    )}
                  </span>
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() =>
                      setCsvName(csvName ? "" : "support-batch-aug.csv")
                    }
                  >
                    {csvName ? "Remove" : "Choose file"}
                  </Button>
                </div>
              )}
              <div className="flex flex-wrap gap-4 text-xs text-muted-foreground">
                <span className="inline-flex items-center gap-1.5">
                  <ShieldCheck className="size-3.5" />
                  Unsubscribes and hard bounces excluded
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <Clock className="size-3.5" />
                  Quiet hours honoured per recipient timezone
                </span>
              </div>
            </div>
          </Panel>

          <Panel
            title="Message"
            description="Rendered through the same dispatcher as single sends"
          >
            <div className="grid gap-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="cmp-tpl">Template</Label>
                  <SimpleSelect
                    id="cmp-tpl"
                    value={templateId}
                    onValueChange={pickTemplate}
                    options={CAMPAIGN_TEMPLATES.map((t) => ({
                      value: t.id,
                      label: t.label,
                    }))}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="cmp-from">From name</Label>
                  <Input
                    id="cmp-from"
                    value={fromName}
                    onChange={(e) => setFromName(e.target.value)}
                  />
                </div>
              </div>
              {tpl.channel === "email" && (
                <div className="grid gap-1.5">
                  <Label htmlFor="cmp-subject">Subject</Label>
                  <Input
                    id="cmp-subject"
                    value={subject}
                    onChange={(e) => setSubject(e.target.value)}
                  />
                </div>
              )}
              <div className="grid gap-1.5">
                <Label htmlFor="cmp-body">Body</Label>
                <Textarea
                  id="cmp-body"
                  rows={6}
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                  className="font-mono text-[13px]"
                />
                <span className="text-xs text-muted-foreground">
                  {tpl.channel === "sms"
                    ? `${Math.ceil(body.length / 160)} SMS segment(s) · ${body.length} chars`
                    : "Variables resolve per recipient at render time"}
                </span>
              </div>
              <Separator />
              <div className="flex items-end gap-2">
                <div className="grid flex-1 gap-1.5">
                  <Label htmlFor="cmp-test">Test send</Label>
                  <Input
                    id="cmp-test"
                    value={testTo}
                    onChange={(e) => setTestTo(e.target.value)}
                  />
                </div>
                <Button size="sm" variant="outline">
                  <FlaskConical className="size-3.5" />
                  Send test
                </Button>
              </div>
            </div>
          </Panel>

          <Panel title="Delivery" description="Rate and schedule">
            <div className="grid gap-4">
              <Tabs value={timing} onValueChange={(v) => setTiming(String(v))}>
                <TabsList>
                  <TabsTrigger value="now">Send now</TabsTrigger>
                  <TabsTrigger value="schedule">Schedule</TabsTrigger>
                </TabsList>
              </Tabs>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="cmp-when">Start at</Label>
                  <Input
                    id="cmp-when"
                    type="datetime-local"
                    value={when}
                    onChange={(e) => setWhen(e.target.value)}
                    disabled={timing === "now"}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="cmp-rate">Throttle (messages / minute)</Label>
                  <Input
                    id="cmp-rate"
                    value={rate}
                    onChange={(e) => setRate(Number(e.target.value) || 0)}
                    className="font-mono"
                  />
                </div>
              </div>
              <div className="text-xs text-muted-foreground">
                {size
                  ? `At ${nf(rate)}/min this batch drains in about ${minutes} minute${minutes === 1 ? "" : "s"}.`
                  : "Pick a list to estimate the send window."}
              </div>
            </div>
          </Panel>
        </div>

        <Panel
          title="Review"
          description="Check before sending"
          className="lg:sticky lg:top-4"
        >
          <div className="grid gap-0.5">
            <div className="text-2xl font-semibold tabular-nums tracking-tight">
              {nf(size)}
            </div>
            <div className="text-xs text-muted-foreground">
              recipients in scope
            </div>
          </div>
          <div className="mt-3">
            <SummaryRow label="Campaign">{name || "Untitled"}</SummaryRow>
            <SummaryRow label="Type">
              {kind === "otp" ? "OTP batch" : "Marketing"}
            </SummaryRow>
            <SummaryRow label="Channel">{tpl.channel}</SummaryRow>
            <SummaryRow label="Template">
              <span className="font-mono text-[13px]">{templateId}</span>
            </SummaryRow>
            <SummaryRow label="List">
              {isCsv ? csvName || "no file" : audience.label}
            </SummaryRow>
            <SummaryRow label="Start">
              {timing === "now" ? "Immediately" : when.replace("T", " ")}
            </SummaryRow>
            <SummaryRow label="Window">
              {size ? `~${minutes} min` : "-"}
            </SummaryRow>
          </div>
          <div className="mt-4 grid gap-2">
            <Button onClick={() => setConfirm(true)} disabled={!size}>
              <Send className="size-4" />
              {timing === "now" ? "Send campaign" : "Schedule campaign"}
            </Button>
            <Button variant="outline" onClick={onBack}>
              Save as draft
            </Button>
          </div>
        </Panel>
      </div>

      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {timing === "now"
                ? `Send to ${nf(size)} recipients?`
                : `Schedule ${nf(size)} sends?`}
            </DialogTitle>
            <DialogDescription>
              This cannot be recalled once the batch starts draining.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2.5 text-sm">
            <SummaryRow label="Template">
              <span className="font-mono text-[13px]">{templateId}</span>
            </SummaryRow>
            <SummaryRow label="Throttle">
              <span className="font-mono text-[13px]">{nf(rate)}/min</span>
            </SummaryRow>
            <SummaryRow label="Estimated window">~{minutes} min</SummaryRow>
          </div>
          <DialogFooter>
            <Button variant="ghost" size="sm" onClick={() => setConfirm(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={() => {
                setConfirm(false);
                onQueue({
                  name: name || "Untitled",
                  kind,
                  channel: tpl.channel,
                  audience: isCsv
                    ? "Upload a recipient list (CSV)"
                    : audience.label,
                  recipients: size,
                  template: tpl.label,
                  status: timing === "now" ? "sending" : "scheduled",
                  when: timing === "now" ? "now" : when.replace("T", " "),
                });
              }}
            >
              <Send className="size-3.5" />
              {timing === "now" ? "Send now" : "Schedule"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
