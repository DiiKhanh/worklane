import { CirclePlus, Send, ShieldCheck, X, type LucideIcon } from "lucide-react";
import type { DeliveryLog, OtpState } from "@/lib/api/types";

/**
 * The lifecycle a code moves through, projected onto three fixed stages.
 * worklane's five states map onto these: a failed send stops at (and fails)
 * the "delivered" stage; sent/expired reach delivery but never verify.
 */
export type StageState = "done" | "failed" | "pending";
export type Stage = {
  key: "requested" | "sent" | "verified";
  label: string;
  detail: string;
  icon: LucideIcon;
  state: StageState;
};

const BASE = [
  {
    key: "requested",
    label: "Requested",
    icon: CirclePlus,
    detail: "Code issued and hashed",
  },
  { key: "sent", label: "Delivered", icon: Send, detail: "Handed to the provider" },
  {
    key: "verified",
    label: "Verified",
    icon: ShieldCheck,
    detail: "Code confirmed, single use",
  },
] as const;

export function lifecycleStages(state: OtpState, log?: DeliveryLog): Stage[] {
  const reached = state === "requested" ? 0 : state === "verified" ? 2 : 1;
  const failed = state === "failed";
  return BASE.map((b, i): Stage => {
    const isFail = failed && i === 1;
    const stageState: StageState = isFail
      ? "failed"
      : i <= reached
        ? "done"
        : "pending";
    return {
      key: b.key,
      label: isFail ? "Delivery failed" : b.label,
      icon: isFail ? X : b.icon,
      detail: isFail ? (log?.error ?? "Delivery failed") : b.detail,
      state: stageState,
    };
  });
}
