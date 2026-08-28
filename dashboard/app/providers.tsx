"use client";

import { ThemeProvider } from "next-themes";
import { useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";
import { QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useUIStore } from "@/lib/store/ui";

/**
 * Client-side app providers. Theme is owned by next-themes (class strategy,
 * dark-first). Server data lives only in the TanStack Query cache.
 */
export function Providers({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [queryClient] = useState(
    () =>
      new QueryClient({
        // A 401 from the live API means the JWT is missing or expired: drop it and send
        // the operator back to sign in (re-login on expiry, per the auth design).
        queryCache: new QueryCache({
          onError: (error) => {
            if (error instanceof Error && /\b401\b/.test(error.message)) {
              localStorage.removeItem("worklane-token");
              useUIStore.getState().setToken("");
              router.replace("/login");
            }
          },
        }),
        defaultOptions: {
          queries: { staleTime: 10_000, refetchOnWindowFocus: false, retry: 1 },
        },
      }),
  );

  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="dark"
      enableSystem={false}
      disableTransitionOnChange
    >
      <QueryClientProvider client={queryClient}>
        <TooltipProvider delay={200}>{children}</TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>
  );
}
