"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useUIStore } from "@/lib/store/ui";

const STORAGE_KEY = "worklane-token";

/**
 * In live mode, redirects to /login when there is no token and rehydrates the store from
 * localStorage on mount. In mock mode it is a no-op so the fixture dashboard needs no login.
 */
export function AuthGuard({ children }: { children: React.ReactNode }) {
  const isLive = process.env.NEXT_PUBLIC_DATA_SOURCE === "live";
  const router = useRouter();
  const token = useUIStore((s) => s.token);
  const setToken = useUIStore((s) => s.setToken);

  useEffect(() => {
    if (!isLive) return;
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved && !token) {
      setToken(saved);
      return;
    }
    if (!saved && !token) router.replace("/login");
  }, [isLive, token, setToken, router]);

  if (!isLive) return <>{children}</>;
  if (!token) return null;
  return <>{children}</>;
}
