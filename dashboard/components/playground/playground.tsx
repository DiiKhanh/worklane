"use client";

import { useState } from "react";
import { Panel } from "@/components/common/panel";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { CHANNELS, type Channel } from "@/lib/schemas";
import { SendForm } from "./send-form";
import { VerifyForm } from "./verify-form";

export function Playground() {
  const [channel, setChannel] = useState<Channel>("email");
  const [prefill, setPrefill] = useState<{ recipient: string; code: string }>();
  const [verifyKey, setVerifyKey] = useState(0);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Label className="text-muted-foreground">Channel</Label>
        <div className="inline-flex rounded-lg border border-border p-0.5">
          {CHANNELS.map((c) => (
            <Button
              key={c}
              type="button"
              size="sm"
              variant={channel === c ? "default" : "ghost"}
              onClick={() => {
                setChannel(c);
                setPrefill(undefined);
              }}
            >
              {c === "sms" ? "SMS" : "Email"}
            </Button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Panel title="1 · Send a code" description="Issues an OTP and returns a request id">
          <SendForm
            key={channel}
            channel={channel}
            onSent={(recipient, code) => {
              setPrefill({ recipient, code });
              setVerifyKey((k) => k + 1);
            }}
          />
        </Panel>

        <Panel title="2 · Verify the code" description="Checks the code, single use">
          <VerifyForm key={`${channel}-${verifyKey}`} channel={channel} initial={prefill} />
        </Panel>
      </div>
    </div>
  );
}
