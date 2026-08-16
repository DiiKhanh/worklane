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
