"use client";

import { useQuery } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useOverview() {
  return useQuery({
    queryKey: qk.overview,
    queryFn: () => getDataSource().getOverview(),
    refetchInterval: 8000,
    // getOverview() throws deterministically in live mode (no /v1/stats endpoint),
    // so retrying the same call 3x per poll is pointless noise - surface the error
    // immediately. The 8s refetch still runs, so it self-heals once a stats endpoint exists.
    retry: false,
  });
}
