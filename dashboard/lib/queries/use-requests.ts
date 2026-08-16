"use client";

import { useQuery } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useRequests() {
  return useQuery({
    queryKey: qk.requests,
    queryFn: () => getDataSource().listRequests(),
  });
}
