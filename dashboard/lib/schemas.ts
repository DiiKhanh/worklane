import { z } from "zod";

export const sendSchema = z.object({
  recipient: z.string().email("Enter a valid email address"),
});
export type SendValues = z.infer<typeof sendSchema>;

export const verifySchema = z.object({
  recipient: z.string().email("Enter a valid email address"),
  code: z
    .string()
    .regex(/^\d{6}$/, "The code is 6 digits"),
});
export type VerifyValues = z.infer<typeof verifySchema>;
