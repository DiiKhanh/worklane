import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, LINK_DISABLED } from "./errors";
import { LiveDataSource } from "./live";

afterEach(() => {
  vi.unstubAllGlobals();
});

function respond(status: number, body: unknown, headers: Record<string, string> = {}) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const ds = () => new LiveDataSource({ baseUrl: "http://gw/", getToken: () => "jwt" });

const wireLink = {
  code: "aB3",
  short_url: "http://127.0.0.1:8890/aB3",
  long_url: "https://example.com/a?b=1#c",
  state: "disabled",
  created_at: "2026-11-20T10:00:00Z",
  disabled_at: "2026-11-21T10:00:00Z",
};

describe("LiveDataSource links", () => {
  it("lists a cursor page with the bearer token and maps the wire shape", async () => {
    const fetchMock = respond(200, { items: [wireLink], next_cursor: "nxt" });
    const page = await ds().listLinks("c/+=");

    expect(fetchMock).toHaveBeenCalledWith(
      "http://gw/v1/links?cursor=c%2F%2B%3D",
      expect.objectContaining({
        method: "GET",
        headers: expect.objectContaining({ Authorization: "Bearer jwt" }),
      }),
    );
    expect(page).toEqual({
      items: [
        {
          code: "aB3",
          shortUrl: "http://127.0.0.1:8890/aB3",
          longUrl: "https://example.com/a?b=1#c",
          state: "disabled",
          createdAt: "2026-11-20T10:00:00Z",
          disabledAt: "2026-11-21T10:00:00Z",
        },
      ],
      nextCursor: "nxt",
    });
  });

  it("requests the first page without a cursor and reports the final page", async () => {
    const fetchMock = respond(200, { items: [], next_cursor: null });
    expect(await ds().listLinks()).toEqual({ items: [], nextCursor: null });
    expect(fetchMock.mock.calls[0][0]).toBe("http://gw/v1/links");
  });

  it("distinguishes a new link (201) from active dedup (200) and sends the URL unchanged", async () => {
    const fetchMock = respond(201, { code: "aB3", short_url: "http://s/aB3" });
    expect(await ds().createLink(" https://example.com/x/")).toEqual({
      code: "aB3",
      shortUrl: "http://s/aB3",
      created: true,
    });
    expect(fetchMock.mock.calls[0][1].body).toBe(JSON.stringify({ long_url: " https://example.com/x/" }));

    respond(200, { code: "aB3", short_url: "http://s/aB3" });
    expect((await ds().createLink("https://example.com/x/")).created).toBe(false);
  });

  it("rejects disabled dedup with a typed LINK_DISABLED error", async () => {
    respond(409, { error: "LINK_DISABLED" });
    const err = await ds().createLink("https://example.com").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: LINK_DISABLED });
  });

  it("carries Retry-After on quota errors", async () => {
    respond(429, { error: "link: rate limited" }, { "Retry-After": "42" });
    await expect(ds().createLink("https://example.com")).rejects.toMatchObject({
      status: 429,
      retryAfterSeconds: 42,
    });
  });

  it("keeps 401 in the message so the session-expiry handler signs out", async () => {
    respond(401, { error: "invalid token" });
    await expect(ds().listLinks()).rejects.toThrow(/\b401\b/);
  });

  it("maps analytics and never invents values on failure", async () => {
    respond(200, {
      total_clicks: 7,
      series: [{ date: "2026-11-28", clicks: 3 }],
      recent: [
        { ts: "2026-11-28T01:00:00Z", referrer_origin: "", device: "mobile" },
        { ts: "2026-11-28T00:00:00Z", referrer_origin: "https://r.example", device: "fridge" },
      ],
    });
    expect(await ds().getLinkAnalytics("aB3")).toEqual({
      totalClicks: 7,
      series: [{ date: "2026-11-28", clicks: 3 }],
      recent: [
        { ts: "2026-11-28T01:00:00Z", referrerOrigin: "", device: "mobile" },
        { ts: "2026-11-28T00:00:00Z", referrerOrigin: "https://r.example", device: "unknown" },
      ],
    });

    respond(503, { error: "link: unavailable" });
    await expect(ds().getLinkAnalytics("aB3")).rejects.toMatchObject({ status: 503 });
  });

  it("disables through the owned-link route and returns the new metadata", async () => {
    const fetchMock = respond(200, wireLink);
    expect((await ds().disableLink("aB3")).state).toBe("disabled");
    expect(fetchMock).toHaveBeenCalledWith("http://gw/v1/links/aB3/disable", expect.objectContaining({ method: "POST" }));
  });
});
