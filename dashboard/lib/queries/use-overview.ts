"use client";

import { useQuery } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useOverview() {
  return useQuery({
    queryKey: qk.overview,
    queryFn: () => getDataSource().getOverview(),
    refetchInterval: 8000,
    // Keep Overview fresh without adding a websocket. Retry is disabled so a transient
    // stats failure surfaces quickly instead of doing three identical aggregate reads per poll.
    retry: false,
  });
}
