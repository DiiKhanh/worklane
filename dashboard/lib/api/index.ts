import type { DataSource } from "./source";
import { MockDataSource } from "./mock";
import { LiveDataSource } from "./live";
import { useUIStore } from "@/lib/store/ui";

export type { DataSource } from "./source";
export * from "./types";

/** Build a data source from the environment. Exported for testing. */
export function createDataSource(): DataSource {
  const kind = process.env.NEXT_PUBLIC_DATA_SOURCE;
  if (kind === "live") {
    return new LiveDataSource({
      baseUrl: process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost:8888",
      getToken: () => useUIStore.getState().token,
    });
  }
  return new MockDataSource();
}

let singleton: DataSource | undefined;

/**
 * The app-wide data source. A single instance is reused so the mock source's
 * in-flight deliveries progress consistently across polling refetches.
 */
export function getDataSource(): DataSource {
  if (!singleton) singleton = createDataSource();
  return singleton;
}
