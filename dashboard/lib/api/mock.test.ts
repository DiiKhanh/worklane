import { describe, it, expect } from "vitest";
import { MockDataSource } from "./mock";

describe("MockDataSource", () => {
  it("returns realistic api keys and masked requests", async () => {
    const ds = new MockDataSource();
    expect((await ds.listApiKeys()).length).toBeGreaterThan(0);
    const reqs = await ds.listRequests();
    expect(reqs.length).toBeGreaterThan(0);
    expect(reqs.every((r) => r.recipient.includes("***"))).toBe(true);
  });

  it("send returns a request id and a 6-digit dev code", async () => {
    const ds = new MockDataSource();
    const r = await ds.send("dev@worklane.io");
    expect(r.requestId).toBeTruthy();
    expect(r.devCode).toMatch(/^\d{6}$/);
  });

  it("verify matches the code from a prior send", async () => {
    const ds = new MockDataSource();
    const sent = await ds.send("dev@worklane.io");
    const good = await ds.verify("dev@worklane.io", sent.devCode!);
    expect(good).toEqual({ ok: true, status: "verified" });
    const bad = await ds.verify("dev@worklane.io", "000000");
    expect(bad.ok).toBe(false);
  });

  it("progresses delivery logs over time (rows appear as they are sent)", async () => {
    const t0 = 1_000_000;
    const early = new MockDataSource({ now: () => t0 });
    const later = new MockDataSource({ now: () => t0 + 60_000 });
    const before = await early.listLogs();
    const after = await later.listLogs();
    expect(before.some((l) => l.status === "sent")).toBe(true);
    expect(after.length).toBeGreaterThanOrEqual(before.length);
  });

  it("overview aggregates are internally consistent", async () => {
    const ds = new MockDataSource({ now: () => 1_700_000_000_000 });
    const o = await ds.getOverview();
    expect(o.verifyRate).toBeGreaterThanOrEqual(0);
    expect(o.verifyRate).toBeLessThanOrEqual(1);
    expect(o.funnel.requested).toBeGreaterThanOrEqual(o.funnel.sent);
    expect(o.funnel.sent).toBeGreaterThanOrEqual(o.funnel.verified);
    expect(o.series.length).toBeGreaterThan(0);
  });
});
