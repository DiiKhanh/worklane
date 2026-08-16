import type { DataSource } from "./source";
import type {
  ApiKey,
  DeliveryLog,
  Overview,
  OtpRequest,
  SendResult,
  VerifyResult,
} from "./types";

type Options = {
  baseUrl: string;
  getToken?: () => string;
};

/**
 * Talks to the real otp-api REST endpoints. Maps snake_case Go JSON to the
 * camelCase dashboard types. The list endpoints omit timestamps today, so
 * `createdAt` falls back to "" until the API exposes it.
 */
export class LiveDataSource implements DataSource {
  private readonly baseUrl: string;
  private readonly getToken: () => string;

  constructor(opts: Options) {
    this.baseUrl = opts.baseUrl.replace(/\/$/, "");
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
      { id: string; tenant_id: string; status: string }[]
    >("/v1/api-keys");
    return rows.map((k) => ({
      id: k.id,
      tenantId: k.tenant_id,
      status: k.status === "revoked" ? "revoked" : "active",
      createdAt: "",
    }));
  }

  async listRequests(): Promise<OtpRequest[]> {
    const rows = await this.get<
      { id: string; recipient: string; channel: string; state: OtpRequest["state"] }[]
    >("/v1/otp/requests");
    return rows.map((r) => ({
      id: r.id,
      recipient: r.recipient,
      channel: r.channel,
      state: r.state,
      createdAt: "",
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
      }[]
    >("/v1/delivery-logs");
    return rows.map((l) => ({
      requestId: l.request_id,
      provider: l.provider,
      status: l.status === "failed" ? "failed" : "sent",
      latencyMs: l.latency_ms,
      error: l.error || undefined,
      createdAt: "",
    }));
  }

  async getOverview(): Promise<Overview> {
    // The API exposes no aggregate endpoint yet (see spec: Overview is mock-only
    // until /v1/stats exists). Fail loudly rather than fabricate live numbers.
    throw new Error(
      "Overview aggregates are not available from the live API yet (no /v1/stats endpoint).",
    );
  }

  async send(recipient: string): Promise<SendResult> {
    const res = await fetch(this.baseUrl + "/v1/otp/send", {
      method: "POST",
      headers: this.authHeaders(),
      body: JSON.stringify({ recipient }),
    });
    if (!res.ok) throw new Error(`send failed: ${res.status}`);
    const body = (await res.json()) as { request_id: string };
    return { requestId: body.request_id };
  }

  async verify(recipient: string, code: string): Promise<VerifyResult> {
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
}
