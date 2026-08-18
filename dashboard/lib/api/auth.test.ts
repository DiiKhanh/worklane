import { describe, it, expect, vi } from "vitest";
import { login } from "./auth";

describe("auth client", () => {
  it("posts credentials and returns the token", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        token: "tok",
        expires_at: "2026-01-01T00:00:00Z",
        user: { id: "u1", email: "a@b.co", tenant_id: "t1" },
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const res = await login("a@b.co", "pw");
    expect(res.token).toBe("tok");
    expect(res.user.tenantId).toBe("t1");
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/auth/login"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("throws on 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 401 }));
    await expect(login("a@b.co", "x")).rejects.toThrow();
  });
});
