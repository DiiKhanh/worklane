"use client";

import { usePathname } from "next/navigation";
import { ExternalLink } from "lucide-react";
import { ThemeToggle } from "./theme-toggle";
import { NAV_ITEMS } from "./nav";
import { Button } from "@/components/ui/button";

function titleFor(pathname: string): string {
  const match = NAV_ITEMS.find((i) =>
    i.href === "/" ? pathname === "/" : pathname.startsWith(i.href),
  );
  return match?.label ?? "worklane";
}

function DataSourceBadge() {
  const live = process.env.NEXT_PUBLIC_DATA_SOURCE === "live";
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card/60 px-2.5 py-1 text-xs font-medium text-muted-foreground">
      <span
        className="size-1.5 rounded-full"
        style={{
          background: live
            ? "var(--state-verified)"
            : "var(--state-requested)",
        }}
      />
      {live ? "Live API" : "Mock data"}
    </span>
  );
}

export function Topbar() {
  const pathname = usePathname();
  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-border bg-background/80 px-5 backdrop-blur-md">
      <h1 className="text-sm font-semibold tracking-tight">
        {titleFor(pathname)}
      </h1>
      <div className="ml-auto flex items-center gap-2">
        <DataSourceBadge />
        <Button
          variant="ghost"
          size="icon"
          aria-label="Documentation"
          nativeButton={false}
          render={
            <a href="https://github.com" target="_blank" rel="noreferrer" />
          }
        >
          <ExternalLink className="size-4" />
        </Button>
        <ThemeToggle />
      </div>
    </header>
  );
}
