"use client";

import { useQuery } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

/** Delivery logs poll on an interval so the screen stays live without websockets. */
export function useLogs() {
  return useQuery({
    queryKey: qk.logs,
    queryFn: () => getDataSource().listLogs(),
    refetchInterval: 4000,
  });
}
