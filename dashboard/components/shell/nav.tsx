"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  KeyRound,
  ListChecks,
  Truck,
  Bell,
  FlaskConical,
  FileText,
  Megaphone,
  Link as LinkIcon,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";

type NavItem = { href: string; label: string; icon: LucideIcon; soon?: boolean };

export const NAV_ITEMS: NavItem[] = [
  { href: "/", label: "Overview", icon: LayoutDashboard },
  { href: "/api-keys", label: "API keys", icon: KeyRound },
  { href: "/requests", label: "OTP requests", icon: ListChecks },
  { href: "/logs", label: "Delivery logs", icon: Truck },
  { href: "/notifications", label: "Notifications", icon: Bell },
  { href: "/playground", label: "Playground", icon: FlaskConical },
  { href: "/templates", label: "Templates", icon: FileText, soon: true },
  { href: "/campaigns", label: "Campaigns", icon: Megaphone, soon: true },
  { href: "/links", label: "Links", icon: LinkIcon, soon: true },
];

export function Nav() {
  const pathname = usePathname();
  return (
    <nav className="flex flex-col gap-0.5 px-2">
      {NAV_ITEMS.map((item) => {
        const active =
          item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
        return (
          <Link
            key={item.href}
            href={item.href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "group relative flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium outline-none transition-colors",
              "focus-visible:ring-2 focus-visible:ring-ring/60",
              active
                ? "text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {active && (
              <span className="absolute inset-0 -z-10 rounded-md bg-accent" />
            )}
            <item.icon
              className={cn(
                "size-4 shrink-0 transition-colors",
                active
                  ? "text-primary"
                  : "text-muted-foreground group-hover:text-foreground",
              )}
            />
            {item.label}
            {item.soon && (
              <span className="ml-auto rounded-full border border-border px-1.5 py-0.5 text-[10px] font-normal text-muted-foreground">
                soon
              </span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}
