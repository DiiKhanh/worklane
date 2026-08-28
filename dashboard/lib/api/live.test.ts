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
