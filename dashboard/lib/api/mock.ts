import type { DataSource } from "./source";
import type {
  ApiKey,
  DeliveryLog,
  Overview,
  OtpRequest,
  OtpState,
  OverviewSeriesPoint,
  SendResult,
  VerifyResult,
} from "./types";

type Clock = () => number;
type Options = { now?: Clock };

const HOUR = 3_600_000;
const PROVIDERS = ["resend", "smtp"] as const;
const DOMAINS = ["gmail.com", "outlook.com", "worklane.io", "proton.me", "acme.co"];
const NAMES = ["daniel", "mai", "khanh", "sora", "wei", "ana", "leo", "priya", "tom", "yuki"];
const ERRORS = [
  "smtp: 550 mailbox unavailable",
  "resend: rate limited (429)",
  "smtp: connection timeout",
];

/** Deterministic PRNG (mulberry32) so fixtures and screenshots are stable. */
function rng(seed: number): () => number {
  let a = seed;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function pick<T>(r: () => number, arr: readonly T[]): T {
  return arr[Math.floor(r() * arr.length)];
}

function mask(email: string): string {
  const [user, domain] = email.split("@");
  return `${user[0] ?? "x"}***@${domain ?? "example.com"}`;
}

function shortId(prefix: string, r: () => number): string {
  const hex = Math.floor(r() * 0xffffffff).toString(16).padStart(8, "0");
  return `${prefix}_${hex}`;
}

// Weighted state distribution that looks like a healthy production stream.
const STATE_WEIGHTS: [OtpState, number][] = [
  ["verified", 62],
  ["sent", 14],
  ["expired", 9],
  ["failed", 8],
  ["requested", 7],
];

function pickState(r: () => number): OtpState {
  const total = STATE_WEIGHTS.reduce((s, [, w]) => s + w, 0);
  let n = r() * total;
  for (const [state, w] of STATE_WEIGHTS) {
    if ((n -= w) <= 0) return state;
  }
  return "sent";
}

type Fixtures = {
  apiKeys: ApiKey[];
  requests: OtpRequest[];
  logs: DeliveryLog[];
};

function buildFixtures(now: number): Fixtures {
  const r = rng(0x9e3779b9);
  const tenants = ["tnt_a1b2c3d4", "tnt_e5f6a7b8"];

  const apiKeys: ApiKey[] = [
    { id: "key_live_9f3a2c11", tenantId: tenants[0], status: "active", createdAt: iso(now - 40 * 24 * HOUR) },
    { id: "key_live_7b1d84ec", tenantId: tenants[0], status: "active", createdAt: iso(now - 12 * 24 * HOUR) },
    { id: "key_test_2c9a51de", tenantId: tenants[1], status: "active", createdAt: iso(now - 6 * 24 * HOUR) },
    { id: "key_live_04ffab73", tenantId: tenants[1], status: "revoked", createdAt: iso(now - 70 * 24 * HOUR) },
  ];

  const requests: OtpRequest[] = [];
  const logs: DeliveryLog[] = [];
  const COUNT = 120;

  for (let i = 0; i < COUNT; i++) {
    // Spread over the last 24h, denser toward "now".
    const ageMs = Math.floor(Math.pow(r(), 1.6) * 24 * HOUR);
    const createdAt = now - ageMs;
    const state = pickState(r);
    const email = `${pick(r, NAMES)}@${pick(r, DOMAINS)}`;
    const id = shortId("req", r);
    requests.push({ id, recipient: mask(email), channel: "email", state, createdAt: iso(createdAt) });

    // A delivery log exists once the dispatcher acted (everything except still-requested).
    if (state !== "requested") {
      const failed = state === "failed";
      const dispatchedAt = createdAt + 200 + Math.floor(r() * 1500);
      logs.push({
        requestId: id,
        provider: pick(r, PROVIDERS),
        status: failed ? "failed" : "sent",
        latencyMs: failed ? 0 : 40 + Math.floor(r() * 900),
        error: failed ? pick(r, ERRORS) : undefined,
        createdAt: iso(dispatchedAt),
      });
    }
  }

  requests.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  logs.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  return { apiKeys, requests, logs };
}

function iso(ms: number): string {
  return new Date(ms).toISOString();
}

function sleep(ms: number): Promise<void> {
  return new Promise((res) => setTimeout(res, ms));
}

/**
 * In-memory data source. Realistic, deterministic fixtures plus a small set of
 * "in-flight" deliveries that surface as time passes, so the live-polling Logs
 * screen visibly updates without any backend.
 */
export class MockDataSource implements DataSource {
  private readonly now: Clock;
  private readonly realClock: boolean;
  private readonly t0: number;
  private readonly fixtures: Fixtures;
  private readonly inflight: DeliveryLog[];
  private readonly codes = new Map<string, string>();

  constructor(opts: Options = {}) {
    this.realClock = !opts.now;
    this.now = opts.now ?? (() => Date.now());
    this.t0 = this.now();
    this.fixtures = buildFixtures(this.t0);
    // Three deliveries that "land" a few seconds apart after construction.
    const r = rng(0x1234abcd);
    this.inflight = [4000, 9000, 16000].map((offset) => ({
      requestId: shortId("req", r),
      provider: pick(r, PROVIDERS),
      status: "sent" as const,
      latencyMs: 40 + Math.floor(r() * 500),
      error: undefined,
      createdAt: iso(this.t0 + offset),
      _sentAt: this.t0 + offset,
    })) as (DeliveryLog & { _sentAt: number })[];
  }

  private async latency() {
    if (this.realClock) await sleep(120);
  }

  async listApiKeys(): Promise<ApiKey[]> {
    await this.latency();
    return this.fixtures.apiKeys;
  }

  async listRequests(): Promise<OtpRequest[]> {
    await this.latency();
    return this.fixtures.requests;
  }

  async listLogs(): Promise<DeliveryLog[]> {
    await this.latency();
    const now = this.now();
    const landed = (this.inflight as (DeliveryLog & { _sentAt: number })[])
      .filter((l) => l._sentAt <= now)
      .map(({ _sentAt, ...log }) => log);
    return [...landed, ...this.fixtures.logs];
  }

  async getOverview(): Promise<Overview> {
    await this.latency();
    return computeOverview(this.fixtures, this.t0);
  }

  async send(recipient: string): Promise<SendResult> {
    await this.latency();
    const r = rng((Date.now() ^ recipient.length) >>> 0);
    const code = String(100000 + Math.floor(r() * 900000));
    this.codes.set(recipient.toLowerCase(), code);
    return { requestId: shortId("req", r), devCode: code };
  }

  async verify(recipient: string, code: string): Promise<VerifyResult> {
    await this.latency();
    const expected = this.codes.get(recipient.toLowerCase());
    if (!expected) return { ok: false, status: "expired" };
    if (expected === code) {
      this.codes.delete(recipient.toLowerCase());
      return { ok: true, status: "verified" };
    }
    return { ok: false, status: "mismatch" };
  }
}

function computeOverview(f: Fixtures, now: number): Overview {
  const dispatched = f.requests.filter((x) => x.state !== "requested" && x.state !== "failed");
  const verified = f.requests.filter((x) => x.state === "verified").length;
  const failed = f.requests.filter((x) => x.state === "failed").length;
  const sentLogs = f.logs.filter((l) => l.status === "sent");
  const latencies = sentLogs.map((l) => l.latencyMs).sort((a, b) => a - b);
  const p50 = latencies.length ? latencies[Math.floor(latencies.length / 2)] : 0;

  const buckets = new Map<string, OverviewSeriesPoint>();
  for (let h = 23; h >= 0; h--) {
    const t = new Date(now - h * HOUR);
    t.setMinutes(0, 0, 0);
    const key = t.toISOString();
    buckets.set(key, { t: key, requested: 0, sent: 0, verified: 0, failed: 0 });
  }
  for (const req of f.requests) {
    const t = new Date(req.createdAt);
    t.setMinutes(0, 0, 0);
    const point = buckets.get(t.toISOString());
    if (!point) continue;
    point.requested += 1;
    if (req.state === "verified") point.verified += 1;
    else if (req.state === "failed") point.failed += 1;
    else if (req.state === "sent" || req.state === "expired") point.sent += 1;
  }

  return {
    sentToday: sentLogs.length,
    verifyRate: dispatched.length ? verified / dispatched.length : 0,
    failed,
    p50LatencyMs: p50,
    series: [...buckets.values()],
    funnel: { requested: f.requests.length, sent: dispatched.length, verified },
  };
}
