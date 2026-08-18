import { z } from "zod";
import { isValidE164 } from "./phone";

export const CHANNELS = ["email", "sms"] as const;
export type Channel = (typeof CHANNELS)[number];

export function recipientSchema(channel: Channel) {
  return channel === "sms"
    ? z.string().refine(isValidE164, "Enter a valid phone number")
    : z.string().email("Enter a valid email address");
}

export const sendSchema = (channel: Channel) =>
  z.object({ recipient: recipientSchema(channel) });
export type SendValues = { recipient: string };

export const verifySchema = (channel: Channel) =>
  z.object({
    recipient: recipientSchema(channel),
    code: z.string().regex(/^\d{6}$/, "The code is 6 digits"),
  });
export type VerifyValues = { recipient: string; code: string };
