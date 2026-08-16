"use client";

import { useQuery } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useApiKeys() {
  return useQuery({
    queryKey: qk.apiKeys,
    queryFn: () => getDataSource().listApiKeys(),
  });
}
