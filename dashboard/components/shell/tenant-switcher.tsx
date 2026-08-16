"use client";

import { useState } from "react";
import { Check, ChevronsUpDown, Building2 } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

const TENANTS = [
  { id: "tnt_a1b2c3d4", name: "Acme Inc" },
  { id: "tnt_e5f6a7b8", name: "Globex" },
];

export function TenantSwitcher() {
  const [selected, setSelected] = useState(TENANTS[0]);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex w-full items-center gap-2.5 rounded-md border border-border bg-card/50 px-3 py-2 text-left outline-none transition-colors hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring/60">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-primary/12 text-primary">
          <Building2 className="size-4" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">
            {selected.name}
          </span>
          <span className="block truncate font-mono text-[11px] text-muted-foreground">
            {selected.id}
          </span>
        </span>
        <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-56">
        <DropdownMenuLabel className="text-xs text-muted-foreground">
          Tenants
        </DropdownMenuLabel>
        {TENANTS.map((t) => (
          <DropdownMenuItem
            key={t.id}
            onClick={() => setSelected(t)}
            className="gap-2"
          >
            <span className="flex size-6 items-center justify-center rounded bg-muted text-muted-foreground">
              <Building2 className="size-3.5" />
            </span>
            <span className="flex-1">
              <span className="block text-sm">{t.name}</span>
              <span className="block font-mono text-[11px] text-muted-foreground">
                {t.id}
              </span>
            </span>
            <Check
              className={cn(
                "size-4",
                selected.id === t.id ? "opacity-100 text-primary" : "opacity-0",
              )}
            />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
