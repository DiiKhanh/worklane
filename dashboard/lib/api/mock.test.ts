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
    const r = await ds.send("dev@worklane.io", "email");
    expect(r.requestId).toBeTruthy();
    expect(r.devCode).toMatch(/^\d{6}$/);
  });

  it("verify matches the code from a prior send", async () => {
    const ds = new MockDataSource();
    const sent = await ds.send("dev@worklane.io", "email");
    const good = await ds.verify("dev@worklane.io", sent.devCode!);
    expect(good).toEqual({ ok: true, status: "verified" });
    const bad = await ds.verify("dev@worklane.io", "000000");
    expect(bad.ok).toBe(false);
  });

  it("send records the chosen channel on the request", async () => {
    const ds = new MockDataSource();
    await ds.send("+84901234567", "sms");
    const requests = await ds.listRequests();
    expect(requests.some((r) => r.channel === "sms")).toBe(true);
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

describe("MockDataSource templates", () => {
  it("previews with sample values through the same var names", async () => {
    const ds = new MockDataSource();
    const out = await ds.previewTemplate({
      channel: "email",
      subject: "Code {{code}}",
      body: "expires {{expiry}}",
    });
    expect(out.subject).toContain("123456");
    expect(out.body).toContain("5 minutes");
  });

  it("lists seeded templates and returns their versions", async () => {
    const ds = new MockDataSource();
    const list = await ds.listTemplates();
    expect(list.length).toBeGreaterThan(0);
    const detail = await ds.getTemplate(list[0].id);
    expect(detail.versions.length).toBeGreaterThan(0);
  });

  it("publishing a version updates the active version", async () => {
    const ds = new MockDataSource();
    const [t] = await ds.listTemplates();
    const draft = await ds.addVersion(t.id, { subject: "Your verification code", body: "New {{code}}", note: "edit" });
    await ds.publishVersion(t.id, draft.id);
    const after = await ds.getTemplate(t.id);
    expect(after.template.activeVersionId).toBe(draft.id);
    expect(after.versions.find((v) => v.id === draft.id)?.status).toBe("published");
  });
});

describe("MockDataSource links", () => {
  it("lists seeded links and returns a 14-day series with recent clicks", async () => {
    const ds = new MockDataSource({ now: () => 1_700_000_000_000 });
    const list = await ds.listLinks();
    expect(list.length).toBeGreaterThan(0);
    const detail = await ds.getLink(list[0].code);
    expect(detail.series).toHaveLength(14);
    expect(detail.recent.length).toBeGreaterThan(0);
    expect(detail.shortUrl.endsWith(`/${detail.code}`)).toBe(true);
  });

  it("creates a link, puts it first in the list and dedups the same URL", async () => {
    const ds = new MockDataSource({ now: () => 1_700_000_000_000 });
    const before = await ds.listLinks();
    const first = await ds.createLink("  https://example.com/a  ");
    expect(first.code).toMatch(/^[0-9a-zA-Z]{10}$/);
    const again = await ds.createLink("https://example.com/a");
    expect(again).toEqual(first);
    const after = await ds.listLinks();
    expect(after).toHaveLength(before.length + 1);
    expect(after[0]).toMatchObject({ code: first.code, target: "https://example.com/a", clicks: 0 });
    expect((await ds.getLink(first.code)).series).toHaveLength(14);
  });

  it("rejects a URL that is not absolute http(s)", async () => {
    const ds = new MockDataSource({ now: () => 1_700_000_000_000 });
    await expect(ds.createLink("javascript:alert(1)")).rejects.toThrow(/invalid url/);
    await expect(ds.createLink("/relative")).rejects.toThrow(/invalid url/);
  });

  it("reports an unknown code as not found", async () => {
    const ds = new MockDataSource({ now: () => 1_700_000_000_000 });
    await expect(ds.getLink("nope")).rejects.toThrow(/not found/);
  });
});
