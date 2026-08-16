import { describe, it, expect, beforeEach } from "vitest";
import { useUIStore } from "./ui";

describe("ui store", () => {
  beforeEach(() => useUIStore.setState({ filters: {}, token: "" }));

  it("holds request filters (ui state only)", () => {
    useUIStore.getState().setFilter("state", "verified");
    expect(useUIStore.getState().filters.state).toBe("verified");
    useUIStore.getState().setFilter("search", "dev@");
    expect(useUIStore.getState().filters.search).toBe("dev@");
  });

  it("clears a filter when set to undefined", () => {
    useUIStore.getState().setFilter("state", "failed");
    useUIStore.getState().setFilter("state", undefined);
    expect(useUIStore.getState().filters.state).toBeUndefined();
  });

  it("stores an in-memory api token", () => {
    useUIStore.getState().setToken("wl_secret");
    expect(useUIStore.getState().token).toBe("wl_secret");
  });
});
