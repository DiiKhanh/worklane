import { describe, it, expect, vi } from "vitest";
import { LiveDataSource } from "./live";

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
