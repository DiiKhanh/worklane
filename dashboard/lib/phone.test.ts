import { describe, it, expect } from "vitest";
import { DIAL_CODES, toE164, isValidE164, parsePhone } from "./phone";

describe("phone", () => {
  it("lists Vietnam first as the default", () => {
    expect(DIAL_CODES[0].code).toBe("+84");
  });

  it("composes E.164 from a dial code and national number, dropping a leading 0", () => {
    expect(toE164("+84", "0901234567")).toBe("+84901234567");
    expect(toE164("+84", "901 234 567")).toBe("+84901234567");
  });

  it("validates E.164 shape", () => {
    expect(isValidE164("+84901234567")).toBe(true);
    expect(isValidE164("0901234567")).toBe(false);
    expect(isValidE164("+0123")).toBe(false);
  });

  it("parses an E.164 value back into a known dial code and national part", () => {
    expect(parsePhone("+84901234567")).toEqual({ dialCode: "+84", national: "901234567" });
    expect(parsePhone("not-a-phone")).toBeNull();
  });
});
