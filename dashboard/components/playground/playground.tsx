"use client";

import { useState } from "react";
import { Panel } from "@/components/common/panel";
import { SendForm } from "./send-form";
import { VerifyForm } from "./verify-form";

export function Playground() {
  const [prefill, setPrefill] = useState<{ recipient: string; code: string }>();
  const [verifyKey, setVerifyKey] = useState(0);

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Panel title="1 · Send a code" description="Issues an OTP and returns a request id">
        <SendForm
          onSent={(recipient, code) => {
            setPrefill({ recipient, code });
            setVerifyKey((k) => k + 1);
          }}
        />
      </Panel>

      <Panel title="2 · Verify the code" description="Checks the code, single use">
        <VerifyForm key={verifyKey} initial={prefill} />
      </Panel>
    </div>
  );
}
