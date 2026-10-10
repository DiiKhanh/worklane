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

// --- Links (URL shortener) ---

export type LinkSummary = {
  code: string;
  shortUrl: string;
  target: string;
  clicks: number; // lifetime
  createdAt: string;
};

export type LinkClick = {
  ts: string;
  ref: string; // referer, "" when the click carried none
  geo: string; // "" until the backend does geo lookup
  device: string;
};

export type LinkDetail = LinkSummary & {
  series: number[]; // daily clicks for the last 14 UTC days, oldest first
  recent: LinkClick[]; // newest first
};

export type ShortenResult = { code: string; shortUrl: string };

// --- Notifications (notification-api log) ---

export type NotificationState = "queued" | "sent" | "failed" | "suppressed";
export type NotificationKind = "transactional" | "marketing";

export type Notification = {
  id: string;
  channel: string;
  recipient: string; // already masked; the log never stores the raw address
  templateId: string;
  kind: NotificationKind;
  state: NotificationState;
  provider: string; // "" until the dispatcher delivered it
  providerMsgId: string;
  latencyMs: number;
  error?: string;
  createdAt: string;
  updatedAt: string;
};

export type NotificationEvent = {
  type: string; // delivered | opened | clicked
  ts: string;
  meta: string;
};

export type NotificationDetail = Notification & {
  events: NotificationEvent[]; // engagement, oldest first
};
