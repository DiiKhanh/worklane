import { describe, it, expect, vi, afterEach } from "vitest";
import { createDataSource } from "./index";
import { MockDataSource } from "./mock";
import { LiveDataSource } from "./live";

afterEach(() => vi.unstubAllEnvs());

describe("createDataSource", () => {
  it("defaults to the mock source", () => {
    vi.stubEnv("NEXT_PUBLIC_DATA_SOURCE", "");
    expect(createDataSource()).toBeInstanceOf(MockDataSource);
  });

  it("returns the mock source when explicitly mock", () => {
    vi.stubEnv("NEXT_PUBLIC_DATA_SOURCE", "mock");
    expect(createDataSource()).toBeInstanceOf(MockDataSource);
  });

  it("returns the live source when configured", () => {
    vi.stubEnv("NEXT_PUBLIC_DATA_SOURCE", "live");
    vi.stubEnv("NEXT_PUBLIC_API_BASE", "http://localhost:8888");
    expect(createDataSource()).toBeInstanceOf(LiveDataSource);
  });
});
