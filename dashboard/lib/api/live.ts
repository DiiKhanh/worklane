import type { DataSource } from "./source";
import type {
  AddVersionInput,
  ApiKey,
  CreateTemplateInput,
  DeliveryLog,
  LinkDetail,
  LinkSummary,
  Overview,
  OtpRequest,
  PreviewInput,
  PreviewResult,
  SendResult,
  ShortenResult,
  Template,
  TemplateChannel,
  TemplateDetail,
  TemplateStatus,
  TemplateVersion,
  VerifyResult,
} from "./types";

// Raw snake_case shapes returned by otp-api's template endpoints.
type templateJSON = {
  id: string;
  name: string;
  channel: string;
  locale: string;
  status: string;
  active_version_id: string;
  updated_at: string;
};
type versionJSON = {
  id: string;
  version_no: number;
  subject: string;
  body: string;
  status: string;
  note: string;
  created_by: string;
  created_at: string;
};

function toTemplate(t: templateJSON): Template {
  return {
    id: t.id,
    name: t.name,
    channel: (t.channel === "sms" ? "sms" : "email") as TemplateChannel,
    locale: t.locale,
    status: (t.status === "archived" ? "archived" : "active") as TemplateStatus,
    activeVersionId: t.active_version_id ?? "",
    updatedAt: t.updated_at ?? "",
  };
}

function toVersion(v: versionJSON): TemplateVersion {
  return {
    id: v.id,
    versionNo: v.version_no,
    subject: v.subject,
    body: v.body,
    status: (v.status as TemplateVersion["status"]) ?? "draft",
    note: v.note ?? "",
    createdBy: v.created_by ?? "",
    createdAt: v.created_at ?? "",
  };
}

// Raw shapes returned by link-svc; `created` and `ts` are RFC 3339 UTC.
type linkJSON = { code: string; target: string; clicks: number; created: string };
type linkDetailJSON = linkJSON & {
  series: number[];
  recent: { ts: string; ref: string; geo: string; device: string }[];
};

type Options = {
  baseUrl: string;
  /** Public short-link host that serves GET /:code, e.g. https://link.example.com. */
  linkBaseUrl?: string;
  getToken?: () => string;
};

/**
 * Talks to the real backend REST endpoints (otp-api for OTP operations,
 * auth-svc for API key management). Maps snake_case Go JSON to the
 * camelCase dashboard types.
 */
export class LiveDataSource implements DataSource {
  private readonly baseUrl: string;
  private readonly linkBaseUrl: string;
  private readonly getToken: () => string;

  constructor(opts: Options) {
    this.baseUrl = opts.baseUrl.replace(/\/$/, "");
    this.linkBaseUrl = (opts.linkBaseUrl ?? this.baseUrl).replace(/\/$/, "");
    this.getToken = opts.getToken ?? (() => "");
  }

  private async get<T>(path: string): Promise<T> {
    const res = await fetch(this.baseUrl + path, {
      headers: this.authHeaders(),
      cache: "no-store",
    });
    if (!res.ok) throw new Error(`GET ${path} failed: ${res.status}`);
    return (await res.json()) as T;
  }

  private authHeaders(): HeadersInit {
    const token = this.getToken();
    return {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    };
  }

  async listApiKeys(): Promise<ApiKey[]> {
    const rows = await this.get<
      { id: string; tenant_id: string; status: string; created_at?: string }[]
    >("/auth/api-keys");
    return rows.map((k) => ({
      id: k.id,
      tenantId: k.tenant_id,
      status: k.status === "revoked" ? "revoked" : "active",
      createdAt: k.created_at ?? "",
    }));
  }

  async listRequests(): Promise<OtpRequest[]> {
    const rows = await this.get<
      { id: string; recipient: string; channel: string; state: OtpRequest["state"]; created_at?: string }[]
    >("/v1/otp/requests");
    return rows.map((r) => ({
      id: r.id,
      recipient: r.recipient,
      channel: r.channel,
      state: r.state,
      createdAt: r.created_at ?? "",
    }));
  }

  async listLogs(): Promise<DeliveryLog[]> {
    const rows = await this.get<
      {
        request_id: string;
        provider: string;
        status: string;
        latency_ms: number;
        error?: string;
        created_at?: string;
      }[]
    >("/v1/delivery-logs");
    return rows.map((l) => ({
      requestId: l.request_id,
      provider: l.provider,
      status: l.status === "failed" ? "failed" : "sent",
      latencyMs: l.latency_ms,
      error: l.error || undefined,
      createdAt: l.created_at ?? "",
    }));
  }

  async getOverview(): Promise<Overview> {
    const body = await this.get<{
      sent_today: number;
      verify_rate: number;
      failed: number;
      p50_latency_ms: number;
      series: {
        t: string;
        requested: number;
        sent: number;
        verified: number;
        failed: number;
      }[];
      funnel: { requested: number; sent: number; verified: number };
    }>("/v1/stats");
    return {
      sentToday: body.sent_today,
      verifyRate: body.verify_rate,
      failed: body.failed,
      p50LatencyMs: body.p50_latency_ms,
      series: body.series,
      funnel: body.funnel,
    };
  }

  async send(recipient: string, channel: string, locale = "en"): Promise<SendResult> {
    const res = await fetch(this.baseUrl + "/v1/otp/send", {
      method: "POST",
      headers: this.authHeaders(),
      body: JSON.stringify({ recipient, channel, locale }),
    });
    if (!res.ok) throw new Error(`send failed: ${res.status}`);
    const body = (await res.json()) as { request_id: string };
    return { requestId: body.request_id };
  }

  async verify(recipient: string, code: string): Promise<VerifyResult> {
    // channel is accepted on the interface for symmetry; the backend keys verification
    // by (tenant, recipient), so it is not sent.
    const res = await fetch(this.baseUrl + "/v1/otp/verify", {
      method: "POST",
      headers: this.authHeaders(),
      body: JSON.stringify({ recipient, code }),
    });
    switch (res.status) {
      case 200:
        return { ok: true, status: "verified" };
      case 401:
        return { ok: false, status: "mismatch" };
      case 410:
        return { ok: false, status: "expired" };
      case 429:
        return { ok: false, status: "locked" };
      default:
        throw new Error(`verify failed: ${res.status}`);
    }
  }

  // --- Template Studio ---

  private async post<T>(path: string, body: unknown): Promise<T> {
    const res = await fetch(this.baseUrl + path, {
      method: "POST",
      headers: this.authHeaders(),
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      let msg = `POST ${path} failed: ${res.status}`;
      try {
        const err = (await res.json()) as { error?: string };
        if (err.error) msg = err.error;
      } catch {
        // keep the status-based message
      }
      throw new Error(msg);
    }
    return (await res.json()) as T;
  }

  async listTemplates(): Promise<Template[]> {
    const rows = await this.get<templateJSON[]>("/v1/templates");
    return rows.map(toTemplate);
  }

  async getTemplate(id: string): Promise<TemplateDetail> {
    const body = await this.get<{ template: templateJSON; versions: versionJSON[] }>(
      `/v1/templates/${encodeURIComponent(id)}`,
    );
    return { template: toTemplate(body.template), versions: (body.versions ?? []).map(toVersion) };
  }

  async createTemplate(input: CreateTemplateInput): Promise<Template> {
    const t = await this.post<templateJSON>("/v1/templates", input);
    return toTemplate(t);
  }

  async addVersion(id: string, input: AddVersionInput): Promise<TemplateVersion> {
    const v = await this.post<versionJSON>(`/v1/templates/${encodeURIComponent(id)}/versions`, input);
    return toVersion(v);
  }

  async publishVersion(id: string, versionId: string): Promise<void> {
    const res = await fetch(
      this.baseUrl +
        `/v1/templates/${encodeURIComponent(id)}/versions/${encodeURIComponent(versionId)}/publish`,
      { method: "POST", headers: this.authHeaders() },
    );
    if (!res.ok) throw new Error(`publish failed: ${res.status}`);
  }

  async previewTemplate(input: PreviewInput): Promise<PreviewResult> {
    return this.post<PreviewResult>("/v1/templates/preview", input);
  }

  // --- Links ---

  private toLink(l: linkJSON): LinkSummary {
    // The list and detail endpoints return no short_url, so it is rebuilt from the
    // configured public host - the same LINK_PUBLIC_BASE link-svc uses on create.
    return {
      code: l.code,
      shortUrl: `${this.linkBaseUrl}/${l.code}`,
      target: l.target,
      clicks: l.clicks ?? 0,
      createdAt: l.created ?? "",
    };
  }

  async listLinks(): Promise<LinkSummary[]> {
    const rows = await this.get<linkJSON[]>("/v1/links");
    return rows.map((l) => this.toLink(l));
  }

  async getLink(code: string): Promise<LinkDetail> {
    const body = await this.get<linkDetailJSON>(`/v1/links/${encodeURIComponent(code)}`);
    return {
      ...this.toLink(body),
      series: body.series ?? [],
      recent: (body.recent ?? []).map((c) => ({
        ts: c.ts ?? "",
        ref: c.ref ?? "",
        geo: c.geo ?? "",
        device: c.device ?? "",
      })),
    };
  }

  async createLink(longUrl: string): Promise<ShortenResult> {
    const body = await this.post<{ code: string; short_url: string }>("/v1/links", {
      long_url: longUrl,
    });
    return { code: body.code, shortUrl: body.short_url };
  }
}
