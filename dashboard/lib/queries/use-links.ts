"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { tenantFromToken } from "@/lib/api/tenant";
import type { Link, LinkPage } from "@/lib/api/types";
import { useUIStore } from "@/lib/store/ui";
import { qk } from "./keys";

const MOCK_TENANT = "mock";

/**
 * The tenant that owns the current session, used to partition link caches.
 * Empty in live mode until a token with a tenant claim is present, which keeps
 * link queries disabled rather than fetching under an unknown identity.
 */
export function useLinkTenant(): string {
  const token = useUIStore((s) => s.token);
  if (process.env.NEXT_PUBLIC_DATA_SOURCE !== "live") return MOCK_TENANT;
  return tenantFromToken(token);
}

export function useLinkPage(cursor: string | undefined) {
  const tenant = useLinkTenant();
  return useQuery({
    queryKey: qk.linkPage(tenant, cursor),
    queryFn: () => getDataSource().listLinks(cursor),
    enabled: !!tenant,
    // Keep the previous page on screen while the next one loads, but never
    // across a tenant change: that would show one tenant's links to another.
    placeholderData: (previous: LinkPage | undefined, previousQuery) =>
      previousQuery?.queryKey[1] === tenant ? previous : undefined,
  });
}

export function useLink(code: string, initial?: Link) {
  const tenant = useLinkTenant();
  return useQuery({
    queryKey: qk.link(tenant, code),
    queryFn: () => getDataSource().getLink(code),
    enabled: !!tenant,
    placeholderData: initial,
  });
}

export function useLinkAnalytics(code: string) {
  const tenant = useLinkTenant();
  return useQuery({
    queryKey: qk.linkAnalytics(tenant, code),
    queryFn: () => getDataSource().getLinkAnalytics(code),
    enabled: !!tenant,
  });
}

export function useCreateLink() {
  const tenant = useLinkTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (longUrl: string) => getDataSource().createLink(longUrl),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.linkPages(tenant) }),
  });
}

export function useDisableLink(code: string) {
  const tenant = useLinkTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => getDataSource().disableLink(code),
    onSuccess: async (link) => {
      qc.setQueryData(qk.link(tenant, code), link);
      await qc.invalidateQueries({ queryKey: qk.linkPages(tenant) });
    },
  });
}
