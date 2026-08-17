import { describe, expect, it } from "vitest";
import { recipientScope, sendWindowMinutes } from "./campaign-math";

describe("recipientScope", () => {
  it("uses audience size when not CSV", () => {
    expect(recipientScope({ isCsv: false, size: 8241, csvLoaded: false })).toBe(
      8241,
    );
  });
  it("is 0 for CSV until a file is loaded, then 218", () => {
    expect(recipientScope({ isCsv: true, size: 0, csvLoaded: false })).toBe(0);
    expect(recipientScope({ isCsv: true, size: 0, csvLoaded: true })).toBe(218);
  });
});

describe("sendWindowMinutes", () => {
  it("returns 0 when size or rate is 0", () => {
    expect(sendWindowMinutes(0, 600)).toBe(0);
    expect(sendWindowMinutes(8241, 0)).toBe(0);
  });
  it("rounds size/rate up to at least 1", () => {
    expect(sendWindowMinutes(8241, 600)).toBe(14);
    expect(sendWindowMinutes(100, 600)).toBe(1);
  });
});
