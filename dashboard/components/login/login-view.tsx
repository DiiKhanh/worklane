"use client";

import { useState } from "react";
import { Send, ShieldCheck } from "lucide-react";
import { Panel } from "@/components/common/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/**
 * Roadmap sign-in: the OTP loop applied to auth. The real dashboard authenticates
 * with a bearer key from an env var and has no session, so this is presentational
 * only - submitting does not sign anyone in.
 */
export function LoginView() {
  const [step, setStep] = useState<"email" | "code">("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");

  return (
    <div className="flex min-h-dvh items-center justify-center bg-background p-6">
      <div className="grid w-[380px] gap-5">
        <span className="flex items-center justify-center gap-2">
          <span className="relative flex size-6 items-center justify-center">
            <span className="absolute inset-0 rounded-[7px] bg-primary/20 blur-[2px]" />
            <span className="relative size-2.5 rounded-full bg-primary" />
          </span>
          <span className="text-[15px] font-semibold tracking-tight">
            worklane
          </span>
        </span>

        <Panel
          title={step === "email" ? "Sign in" : "Enter your code"}
          description={
            step === "email"
              ? "We send a one-time code to your work email."
              : `Sent to ${email}. It expires in 5 minutes.`
          }
        >
          {step === "email" ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (email) setStep("code");
              }}
              className="grid gap-4"
            >
              <div className="grid gap-1.5">
                <Label htmlFor="login-email">Work email</Label>
                <Input
                  id="login-email"
                  placeholder="you@company.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>
              <Button type="submit" className="w-full">
                <Send className="size-4" />
                Send code
              </Button>
            </form>
          ) : (
            <form
              onSubmit={(e) => e.preventDefault()}
              className="grid gap-4"
            >
              <div className="grid gap-1.5">
                <Label htmlFor="login-code">6-digit code</Label>
                <Input
                  id="login-code"
                  inputMode="numeric"
                  maxLength={6}
                  placeholder="000000"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  className="font-mono tracking-[0.3em]"
                />
              </div>
              <Button type="submit" className="w-full">
                <ShieldCheck className="size-4" />
                Verify and continue
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setStep("email")}
              >
                Use a different email
              </Button>
            </form>
          )}
        </Panel>

        <p className="m-0 text-center text-xs text-muted-foreground">
          Roadmap screen - the repository authenticates with a bearer key, not a
          session.
        </p>
      </div>
    </div>
  );
}
