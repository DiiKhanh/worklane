"use client";

import { useQuery } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

/** Polls like the delivery logs: a queued notification turns sent or failed within seconds. */
export function useNotifications() {
  return useQuery({
    queryKey: qk.notifications,
    queryFn: () => getDataSource().listNotifications(),
    refetchInterval: 4000,
  });
}

export function useNotification(id: string) {
  return useQuery({
    queryKey: qk.notification(id),
    queryFn: () => getDataSource().getNotification(id),
    enabled: !!id,
  });
}
