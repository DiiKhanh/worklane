import { afterEach, describe, it, expect, vi } from "vitest";
import { LiveDataSource } from "./live";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("LiveDataSource.listApiKeys", () => {
  it("calls /auth/api-keys with the bearer token and maps the response", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { id: "k1", tenant_id: "t1", status: "active", created_at: "2026-08-19T00:00:00Z" },
        { id: "k2", tenant_id: "t1", status: "revoked" },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);

    const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
    const keys = await ds.listApiKeys();

    // Hits the identity service, not the old /v1/api-keys route.
    expect(fetchMock).toHaveBeenCalledWith(
      "http://x/auth/api-keys",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer jwt" }),
      }),
    );
    expect(keys).toEqual([
      { id: "k1", tenantId: "t1", status: "active", createdAt: "2026-08-19T00:00:00Z" },
      { id: "k2", tenantId: "t1", status: "revoked", createdAt: "" },
    ]);
  });
});

describe("LiveDataSource.getOverview", () => {
  it("calls /v1/stats with the bearer token and maps snake_case fields", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        sent_today: 2,
        verify_rate: 0.5,
        failed: 1,
        p50_latency_ms: 123,
        series: [
          {
            t: "2026-08-28T08:00:00Z",
            requested: 1,
            sent: 2,
            verified: 1,
            failed: 0,
          },
        ],
        funnel: { requested: 4, sent: 3, verified: 1 },
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
    const overview = await ds.getOverview();

    expect(fetchMock).toHaveBeenCalledWith(
      "http://x/v1/stats",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer jwt" }),
      }),
    );
    expect(overview).toEqual({
      sentToday: 2,
      verifyRate: 0.5,
      failed: 1,
      p50LatencyMs: 123,
      series: [
        {
          t: "2026-08-28T08:00:00Z",
          requested: 1,
          sent: 2,
          verified: 1,
          failed: 0,
        },
      ],
      funnel: { requested: 4, sent: 3, verified: 1 },
    });
  });
});

describe("LiveDataSource templates", () => {
  it("previewTemplate POSTs to /v1/templates/preview and returns the rendered result", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ subject: "Code 123456", body: "expires 5 minutes" }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
    const out = await ds.previewTemplate({ channel: "email", subject: "Code {{code}}", body: "expires {{expiry}}" });
    expect(out.subject).toBe("Code 123456");
    expect(out.body).toBe("expires 5 minutes");
    expect(fetchMock.mock.calls[0][0]).toContain("/v1/templates/preview");
  });

  it("listTemplates maps snake_case JSON to camelCase", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { id: "t1", name: "OTP email", channel: "email", locale: "en", status: "active", active_version_id: "v1", updated_at: "2026-08-29T00:00:00Z" },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
    const [t] = await ds.listTemplates();
    expect(t).toEqual({ id: "t1", name: "OTP email", channel: "email", locale: "en", status: "active", activeVersionId: "v1", updatedAt: "2026-08-29T00:00:00Z" });
  });

  it("previewTemplate surfaces the backend error message on 400", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: async () => ({ error: "templating: unknown variable: \"name\"" }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
    await expect(ds.previewTemplate({ channel: "email", subject: "s", body: "{{name}}" })).rejects.toThrow(/unknown variable/);
  });
});

describe("LiveDataSource.send locale", () => {
  it("posts the chosen locale in the send body", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ request_id: "r1" }) });
    vi.stubGlobal("fetch", fetchMock);
    const ds = new LiveDataSource({ baseUrl: "http://x", getToken: () => "jwt" });
    await ds.send("d@e.com", "email", "vi");
    const bodySent = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(bodySent.locale).toBe("vi");
    expect(bodySent.channel).toBe("email");
  });
});

describe("LiveDataSource links", () => {
  const opts = { baseUrl: "http://x", linkBaseUrl: "http://link.x/", getToken: () => "jwt" };

  it("listLinks calls /v1/links with the bearer token and builds short URLs from the link host", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { code: "abc123XYZ0", target: "https://example.com/a", clicks: 3, created: "2026-10-01T00:00:00Z" },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    const links = await new LiveDataSource(opts).listLinks();
    expect(fetchMock).toHaveBeenCalledWith(
      "http://x/v1/links",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer jwt" }),
      }),
    );
    expect(links).toEqual([
      {
        code: "abc123XYZ0",
        shortUrl: "http://link.x/abc123XYZ0",
        target: "https://example.com/a",
        clicks: 3,
        createdAt: "2026-10-01T00:00:00Z",
      },
    ]);
  });

  it("getLink maps the detail shape, including series and recent clicks", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        code: "abc",
        target: "https://example.com/a",
        clicks: 2,
        created: "2026-10-01T00:00:00Z",
        series: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1],
        recent: [{ ts: "2026-10-10T08:00:00Z", ref: "https://mail.example/", geo: "", device: "iOS" }],
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const detail = await new LiveDataSource(opts).getLink("abc");
    expect(fetchMock.mock.calls[0][0]).toBe("http://x/v1/links/abc");
    expect(detail).toEqual({
      code: "abc",
      shortUrl: "http://link.x/abc",
      target: "https://example.com/a",
      clicks: 2,
      createdAt: "2026-10-01T00:00:00Z",
      series: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1],
      recent: [{ ts: "2026-10-10T08:00:00Z", ref: "https://mail.example/", geo: "", device: "iOS" }],
    });
  });

  it("getLink rejects when the code is unknown to the tenant", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 404 }));
    await expect(new LiveDataSource(opts).getLink("nope")).rejects.toThrow(/404/);
  });

  it("createLink POSTs long_url and returns the backend's short_url", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ code: "abc", short_url: "https://link.example.com/abc" }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const out = await new LiveDataSource(opts).createLink("https://example.com/a");
    expect(fetchMock.mock.calls[0][0]).toBe("http://x/v1/links");
    expect(fetchMock.mock.calls[0][1].method).toBe("POST");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ long_url: "https://example.com/a" });
    expect(out).toEqual({ code: "abc", shortUrl: "https://link.example.com/abc" });
  });

  it("createLink surfaces the backend error message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, status: 400, json: async () => ({ error: "invalid url" }) }),
    );
    await expect(new LiveDataSource(opts).createLink("nope")).rejects.toThrow(/invalid url/);
  });
});
