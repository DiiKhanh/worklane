import { act, render, screen, waitFor } from "@testing-library/react";
import { useQuery } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useUIStore } from "@/lib/store/ui";
import { Providers } from "./providers";

vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: vi.fn() }) }));

// A screen whose cache key does not include the identity, like most dashboard screens.
function Probe() {
  const { data } = useQuery({
    queryKey: ["probe"],
    queryFn: async () => `data for ${useUIStore.getState().token}`,
    staleTime: Infinity,
  });
  return <p>{data ?? "loading"}</p>;
}

afterEach(() => useUIStore.getState().setToken(""));

describe("Providers", () => {
  it("drops cached server data when the session token changes", async () => {
    act(() => useUIStore.getState().setToken("session-a"));
    const ui = (show: boolean) => <Providers>{show ? <Probe /> : null}</Providers>;
    const { rerender } = render(ui(true));
    expect(await screen.findByText("data for session-a")).toBeInTheDocument();

    // Leave the screen, switch identity, return: the same client must not serve
    // the cached (never-stale) response from the previous session.
    rerender(ui(false));
    act(() => useUIStore.getState().setToken("session-b"));
    rerender(ui(true));
    await waitFor(() => expect(screen.getByText("data for session-b")).toBeInTheDocument());
    expect(screen.queryByText("data for session-a")).not.toBeInTheDocument();
  });
});
