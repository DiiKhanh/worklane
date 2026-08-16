import { describe, it, expect } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useLogs } from "./use-logs";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  Wrapper.displayName = "TestQueryWrapper";
  return Wrapper;
}

describe("useLogs", () => {
  it("loads delivery logs from the data source", async () => {
    const { result } = renderHook(() => useLogs(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.data?.length).toBeGreaterThan(0));
    expect(result.current.data?.[0]).toHaveProperty("requestId");
  });
});
