import { describe, expect, it } from "vitest";
import { tenantFromToken } from "./tenant";

function jwt(claims: object): string {
  const b64url = (s: string) => btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `${b64url('{"alg":"EdDSA"}')}.${b64url(JSON.stringify(claims))}.sig`;
}

describe("tenantFromToken", () => {
  it("reads the tenant_id claim", () => {
    expect(tenantFromToken(jwt({ sub: "u1", tenant_id: "tnt_a~?>" }))).toBe("tnt_a~?>");
  });

  it("returns empty for missing or malformed tokens", () => {
    expect(tenantFromToken("")).toBe("");
    expect(tenantFromToken("opaque-api-key")).toBe("");
    expect(tenantFromToken("a.!!!.c")).toBe("");
    expect(tenantFromToken(jwt({ sub: "u1" }))).toBe("");
  });
});
