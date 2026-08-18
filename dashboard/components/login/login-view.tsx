"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { LogIn } from "lucide-react";
import { login } from "@/lib/api/auth";
import { useUIStore } from "@/lib/store/ui";
import { Panel } from "@/components/common/panel";
import { LogoMark } from "@/components/shell/logo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const STORAGE_KEY = "worklane-token";

/**
 * Working sign-in: email + password against auth-svc. On success the JWT is stored (UI
 * store + localStorage) and the operator is sent to the dashboard.
 */
export function LoginView() {
  const router = useRouter();
  const setToken = useUIStore((s) => s.setToken);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const res = await login(email, password);
      setToken(res.token);
      localStorage.setItem(STORAGE_KEY, res.token);
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center bg-background p-6">
      <div className="grid w-[380px] gap-5">
        <span className="flex items-center justify-center gap-2">
          <LogoMark className="size-6" />
          <span className="text-[15px] font-semibold tracking-tight">worklane</span>
        </span>
        <Panel title="Sign in" description="Use your worklane operator account.">
          <form onSubmit={onSubmit} className="grid gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="login-email">Email</Label>
              <Input
                id="login-email"
                type="email"
                autoComplete="username"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@company.com"
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="login-password">Password</Label>
              <Input
                id="login-password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {error && (
              <p role="alert" className="m-0 text-xs text-[var(--state-failed)]">
                {error}
              </p>
            )}
            <Button type="submit" className="w-full" disabled={busy}>
              <LogIn className="size-4" />
              {busy ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </Panel>
      </div>
    </div>
  );
}
