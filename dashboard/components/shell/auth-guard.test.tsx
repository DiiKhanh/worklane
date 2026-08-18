import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render } from "@testing-library/react";
import { AuthGuard } from "./auth-guard";
import { useUIStore } from "@/lib/store/ui";

const replace = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));

describe("AuthGuard (live mode)", () => {
  beforeEach(() => {
    replace.mockClear();
    localStorage.clear();
    useUIStore.getState().setToken("");
    vi.stubEnv("NEXT_PUBLIC_DATA_SOURCE", "live");
  });
  afterEach(() => vi.unstubAllEnvs());

  it("redirects to /login when there is no token", () => {
    render(
      <AuthGuard>
        <div>secret</div>
      </AuthGuard>,
    );
    expect(replace).toHaveBeenCalledWith("/login");
  });

  it("renders children when a token is present", () => {
    useUIStore.getState().setToken("tok");
    const { getByText } = render(
      <AuthGuard>
        <div>secret</div>
      </AuthGuard>,
    );
    expect(getByText("secret")).toBeTruthy();
  });
});
