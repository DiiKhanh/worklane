"use client";

import { useQuery } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useOverview() {
  return useQuery({
    queryKey: qk.overview,
    queryFn: () => getDataSource().getOverview(),
    refetchInterval: 8000,
  });
}
