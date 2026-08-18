"use client";

import { useEffect, useState } from "react";
import { KeyRound, Check } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useUIStore } from "@/lib/store/ui";
import { Input } from "@/components/ui/input";

const STORAGE_KEY = "worklane-api-key";

/**
 * API-key entry for live mode. The key is the bearer token the LiveDataSource
 * sends on every request; it is held in the UI store (persisted to localStorage
 * so a reload keeps it) and never leaves the browser except as an Authorization
 * header to the API. Rendered only when the dashboard runs against the live API.
 */
export function ApiKeyField() {
  const token = useUIStore((s) => s.token);
  const setToken = useUIStore((s) => s.setToken);
  const queryClient = useQueryClient();
  const [focused, setFocused] = useState(false);

  // Rehydrate the key from localStorage once on mount (browser-only concern kept
  // out of the store, so the store stays pure and SSR-safe).
  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) setToken(saved);
  }, [setToken]);

  const update = (value: string) => {
    setToken(value);
    if (value) localStorage.setItem(STORAGE_KEY, value);
    else localStorage.removeItem(STORAGE_KEY);
  };

  return (
    <div className="relative hidden items-center sm:flex">
      <KeyRound className="pointer-events-none absolute left-2.5 size-3.5 text-muted-foreground" />
      <Input
        type="password"
        value={token}
        onChange={(e) => update(e.target.value)}
        onFocus={() => setFocused(true)}
        onBlur={() => {
          setFocused(false);
          // Refetch data with the new key so it loads without a page reload.
          queryClient.invalidateQueries();
        }}
        placeholder="Paste API key"
        aria-label="API key"
        autoComplete="off"
        className="h-8 w-44 pl-8 pr-7 font-mono text-xs"
      />
      {token && !focused && (
        <Check className="pointer-events-none absolute right-2.5 size-3.5 text-[var(--state-verified)]" />
      )}
    </div>
  );
}
