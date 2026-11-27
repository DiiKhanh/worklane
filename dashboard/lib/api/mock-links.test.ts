import { describe, expect, it } from "vitest";
import { MockDataSource } from "./mock";

describe("MockDataSource links", () => {
  const now = () => Date.UTC(2026, 10, 28, 12);

  it("dedups exact destinations and refuses disabled ones", async () => {
    const ds = new MockDataSource({ now });
    const first = await ds.createLink("https://example.com/a/");
    expect(first.created).toBe(true);
    expect(await ds.createLink("https://example.com/a/")).toEqual({ ...first, created: false });
    expect((await ds.createLink("https://example.com/a")).code).not.toBe(first.code);

    await ds.disableLink(first.code);
    await expect(ds.createLink("https://example.com/a/")).rejects.toMatchObject({ status: 409, code: "LINK_DISABLED" });
  });

  it("pages newest first with a cursor", async () => {
    const ds = new MockDataSource({ now });
    for (let i = 0; i < 25; i++) await ds.createLink(`https://example.com/${i}`);
    const p1 = await ds.listLinks();
    expect(p1.items).toHaveLength(20);
    expect(p1.items[0].longUrl).toBe("https://example.com/24");
    const p2 = await ds.listLinks(p1.nextCursor!);
    expect(p2.nextCursor).toBeNull();
    expect(new Set([...p1.items, ...p2.items].map((l) => l.code)).size).toBe(p1.items.length + p2.items.length);
  });

  it("returns 14 UTC days ending today", async () => {
    const ds = new MockDataSource({ now });
    const { code } = await ds.createLink("https://example.com/new");
    const a = await ds.getLinkAnalytics(code);
    expect(a.series).toHaveLength(14);
    expect(a.series.at(-1)?.date).toBe("2026-11-28");
    expect(a.series[0].date).toBe("2026-11-15");
    expect(a).toMatchObject({ totalClicks: 0, recent: [] });
  });
});
