"use client";

import { Building2 } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { me } from "@/lib/api/auth";
import { useUIStore } from "@/lib/store/ui";
import { TenantSwitcher } from "./tenant-switcher";

/**
 * Shows the signed-in operator's identity in live mode (from /auth/me). In mock mode it
 * falls back to the fixture TenantSwitcher so the demo dashboard is unchanged.
 */
export function SidebarIdentity() {
  const isLive = process.env.NEXT_PUBLIC_DATA_SOURCE === "live";
  if (!isLive) return <TenantSwitcher />;
  return <LiveIdentity />;
}

function LiveIdentity() {
  const token = useUIStore((s) => s.token);
  const { data } = useQuery({
    queryKey: ["me", token],
    queryFn: () => me(token),
    enabled: !!token,
  });
  return (
    <div className="flex w-full items-center gap-2.5 rounded-md border border-border bg-card/50 px-3 py-2">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-primary/12 text-primary">
        <Building2 className="size-4" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{data?.email ?? "..."}</span>
        <span className="block truncate font-mono text-[11px] text-muted-foreground">
          {data?.tenantId ?? ""}
        </span>
      </span>
    </div>
  );
}
