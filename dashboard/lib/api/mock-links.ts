import { ApiError, LINK_DISABLED } from "./errors";
import type { CreateLinkResult, Link, LinkAnalytics, LinkDevice, LinkPage } from "./types";

const DAY = 86_400_000;
const PAGE_SIZE = 20;
const SERIES_DAYS = 14;
// A reserved example host, so mock short URLs can never be mistaken for a real one.
const MOCK_PUBLIC_BASE = "https://short.example";
const BASE62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
const DEVICES: readonly LinkDevice[] = ["mobile", "desktop", "mobile", "tablet", "unknown"];
const REFERRERS = ["https://mail.example", "", "https://chat.example", "https://news.example"];

type StoredLink = Link & { clicks: number[] }; // daily clicks, oldest first, today last

function isoDay(ms: number): string {
  return new Date(ms).toISOString().slice(0, 10);
}

function startOfUtcDay(ms: number): number {
  return Math.floor(ms / DAY) * DAY;
}

/**
 * In-memory link-svc for the mock data source. Mirrors the API semantics the
 * dashboard depends on: exact-string dedup, permanent disable, 409 on disabled
 * dedup, newest-first cursor pages and a 14-day UTC series.
 */
export class MockLinks {
  private readonly links: StoredLink[] = [];
  private seq = 0;

  constructor(private readonly now: () => number) {
    const t0 = now();
    const seeds: [string, number[], number, boolean][] = [
      ["https://worklane.example/status", [0, 1, 0, 2, 1, 0, 3, 1, 0, 0, 2, 1, 0, 1], 5 * DAY, true],
      ["https://worklane.example/verify/help", [0, 0, 1, 3, 2, 4, 3, 5, 6, 4, 7, 5, 8, 6], 3 * DAY, false],
      ["https://worklane.example/settings/notifications", [2, 3, 5, 4, 6, 8, 7, 9, 12, 10, 14, 13, 16, 18], 2 * 3_600_000, false],
    ];
    for (const [longUrl, clicks, age, disabled] of seeds) {
      const link = this.insert(longUrl, t0 - age);
      link.clicks = clicks;
      if (disabled) {
        link.state = "disabled";
        link.disabledAt = new Date(t0 - DAY).toISOString();
      }
    }
  }

  private insert(longUrl: string, createdMs: number): StoredLink {
    const n = 916_132_832 + ++this.seq * 7_919_317; // spread codes like Snowflake ids do
    let code = "";
    for (let v = n; v > 0; v = Math.floor(v / 62)) code = BASE62[v % 62] + code;
    const link: StoredLink = {
      code,
      shortUrl: `${MOCK_PUBLIC_BASE}/${code}`,
      longUrl,
      state: "active",
      createdAt: new Date(createdMs).toISOString(),
      disabledAt: null,
      clicks: new Array(SERIES_DAYS).fill(0),
    };
    this.links.unshift(link);
    return link;
  }

  private find(code: string): StoredLink {
    const link = this.links.find((l) => l.code === code);
    if (!link) throw new ApiError(404, "link: not found");
    return link;
  }

  private static metadata(link: StoredLink): Link {
    const { code, shortUrl, longUrl, state, createdAt, disabledAt } = link;
    return { code, shortUrl, longUrl, state, createdAt, disabledAt };
  }

  list(cursor?: string): LinkPage {
    const start = cursor ? this.links.findIndex((l) => l.code === cursor) + 1 : 0;
    if (cursor && start === 0) throw new ApiError(400, "link: invalid request");
    const page = this.links.slice(start, start + PAGE_SIZE);
    const more = start + PAGE_SIZE < this.links.length;
    return { items: page.map(MockLinks.metadata), nextCursor: more ? page[page.length - 1].code : null };
  }

  get(code: string): Link {
    return MockLinks.metadata(this.find(code));
  }

  analytics(code: string): LinkAnalytics {
    const link = this.find(code);
    const today = startOfUtcDay(this.now());
    const series = link.clicks.map((clicks, i) => ({
      date: isoDay(today - (SERIES_DAYS - 1 - i) * DAY),
      clicks,
    }));
    const totalClicks = link.clicks.reduce((a, b) => a + b, 0);
    const recent = Array.from({ length: Math.min(totalClicks, 20) }, (_, i) => ({
      ts: new Date(this.now() - (i * 47 + 3) * 60_000).toISOString(),
      referrerOrigin: REFERRERS[i % REFERRERS.length],
      device: DEVICES[i % DEVICES.length],
    }));
    return { totalClicks, series, recent };
  }

  create(longUrl: string): CreateLinkResult {
    let parsed: URL | undefined;
    try {
      parsed = new URL(longUrl);
    } catch {
      parsed = undefined;
    }
    if (!parsed || (parsed.protocol !== "http:" && parsed.protocol !== "https:")) {
      throw new ApiError(400, "link: invalid request");
    }
    const existing = this.links.find((l) => l.longUrl === longUrl);
    if (existing?.state === "disabled") throw new ApiError(409, LINK_DISABLED);
    if (existing) return { code: existing.code, shortUrl: existing.shortUrl, created: false };
    const link = this.insert(longUrl, this.now());
    return { code: link.code, shortUrl: link.shortUrl, created: true };
  }

  disable(code: string): Link {
    const link = this.find(code);
    if (link.state === "active") {
      link.state = "disabled";
      link.disabledAt = new Date(this.now()).toISOString();
    }
    return MockLinks.metadata(link);
  }
}
