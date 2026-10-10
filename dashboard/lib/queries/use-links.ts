"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useLinks() {
  return useQuery({ queryKey: qk.links, queryFn: () => getDataSource().listLinks() });
}

export function useLink(code: string) {
  return useQuery({
    queryKey: qk.link(code),
    queryFn: () => getDataSource().getLink(code),
    enabled: !!code,
  });
}

export function useCreateLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (longUrl: string) => getDataSource().createLink(longUrl),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.links }),
  });
}
