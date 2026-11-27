/**
 * Dashboard-facing types. These mirror the otp-api Go DTOs, converted to camelCase.
 * The Go list endpoints omit timestamps today; `createdAt` is populated by the live
 * adapter when present and by the mock source always, so the UI can sort and show "when".
 */

export type OtpState = "requested" | "sent" | "verified" | "failed" | "expired";
export type DeliveryStatus = "sent" | "failed";
export type ApiKeyStatus = "active" | "revoked";

export type ApiKey = {
  id: string;
  tenantId: string;
  status: ApiKeyStatus;
  createdAt: string;
};

export type OtpRequest = {
  id: string;
  recipient: string; // already masked, e.g. d***@gmail.com
  channel: string;
  state: OtpState;
  createdAt: string;
};

export type DeliveryLog = {
  requestId: string;
  provider: string; // resend | smtp
  status: DeliveryStatus;
  latencyMs: number;
  error?: string;
  createdAt: string;
};

export type OverviewSeriesPoint = {
  t: string; // ISO hour bucket
  requested: number;
  sent: number;
  verified: number;
  failed: number;
};

export type Overview = {
  sentToday: number;
  verifyRate: number; // 0..1
  failed: number;
  p50LatencyMs: number;
  series: OverviewSeriesPoint[];
  funnel: { requested: number; sent: number; verified: number };
};

export type SendResult = {
  requestId: string;
  devCode?: string; // present only in mock / dev, mirrors what MailHog would show
};

export type VerifyOutcome = "verified" | "mismatch" | "expired" | "locked";
export type VerifyResult = {
  ok: boolean;
  status: VerifyOutcome;
};

// --- Template Studio ---

export type TemplateChannel = "email" | "sms";
export type TemplateStatus = "active" | "archived";
export type VersionStatus = "draft" | "published" | "superseded";

export type Template = {
  id: string;
  name: string;
  channel: TemplateChannel;
  locale: string;
  status: TemplateStatus;
  activeVersionId: string;
  updatedAt: string;
};

export type TemplateVersion = {
  id: string;
  versionNo: number;
  subject: string;
  body: string;
  status: VersionStatus;
  note: string;
  createdBy: string;
  createdAt: string;
};

export type TemplateDetail = { template: Template; versions: TemplateVersion[] };

export type PreviewInput = { channel: string; subject: string; body: string };
export type PreviewResult = { subject: string; body: string };
export type CreateTemplateInput = {
  name: string;
  channel: TemplateChannel;
  locale: string;
  subject: string;
  body: string;
  note?: string;
};
export type AddVersionInput = { subject: string; body: string; note?: string };

// --- Links (link-svc) ---

export type LinkState = "active" | "disabled";
export type LinkDevice = "desktop" | "mobile" | "tablet" | "unknown";

/** Owned link metadata. Independent of analytics availability. */
export type Link = {
  code: string;
  shortUrl: string; // built by link-svc from its configured public base
  longUrl: string;
  state: LinkState;
  createdAt: string; // RFC3339 UTC
  disabledAt: string | null;
};

/** One cursor page, newest first. `nextCursor` is null on the final page. */
export type LinkPage = { items: Link[]; nextCursor: string | null };

/** `created` is false when an active link for the same destination was returned. */
export type CreateLinkResult = { code: string; shortUrl: string; created: boolean };

export type LinkVisit = {
  ts: string;
  referrerOrigin: string; // empty when unknown
  device: LinkDevice;
};

export type LinkAnalytics = {
  totalClicks: number; // lifetime recorded redirects
  series: { date: string; clicks: number }[]; // 14 UTC days, oldest first, today last
  recent: LinkVisit[]; // up to 20 retained visits, newest first
};
