import { describe, expect, it } from "vitest";
import { lifecycleStages } from "./lifecycle";

const mark = (s: ReturnType<typeof lifecycleStages>) => s.map((x) => x.state);

describe("lifecycleStages", () => {
  it("requested: only the first stage is done", () => {
    expect(mark(lifecycleStages("requested"))).toEqual([
      "done",
      "pending",
      "pending",
    ]);
  });

  it("failed: delivery stage marked failed, uses log error as detail", () => {
    const s = lifecycleStages("failed", {
      requestId: "r",
      provider: "smtp",
      status: "failed",
      latencyMs: 0,
      error: "smtp: 550 mailbox unavailable",
      createdAt: "",
    });
    expect(mark(s)).toEqual(["done", "failed", "pending"]);
    expect(s[1].detail).toContain("550");
  });

  it("verified: all stages done", () => {
    expect(mark(lifecycleStages("verified"))).toEqual(["done", "done", "done"]);
  });

  it("sent and expired reach the delivery stage but not verified", () => {
    expect(mark(lifecycleStages("sent"))).toEqual(["done", "done", "pending"]);
    expect(mark(lifecycleStages("expired"))).toEqual([
      "done",
      "done",
      "pending",
    ]);
  });
});
