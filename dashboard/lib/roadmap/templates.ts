/**
 * Local fixtures for the Templates roadmap screen. Not wired to any DataSource -
 * these are UI-only, ported verbatim from the design-system kit.
 */
import type { OtpState } from "@/lib/api/types";

export type TemplateVersion = { v: number; when: string; by: string; note: string };
export type TemplateSend = {
  id: string;
  recipient: string;
  state: OtpState;
  when: string;
};
export type Template = {
  id: string;
  name: string;
  channel: "email" | "sms";
  locale: string;
  version: number;
  status: "active" | "expired";
  updated: string;
  subject: string;
  body: string;
  versions: TemplateVersion[];
  sends: TemplateSend[];
};

const OTP_BODY =
  'Your worklane verification code is {{code}}. It expires in 5 minutes.\n\nIf you did not request it, ignore this message.\n\nManage notifications: {{link "https://worklane.io/settings"}}';
const WELCOME_BODY =
  'Hi {{name}},\n\nWelcome to worklane. Confirm your email to finish setting up: {{link "https://worklane.io/confirm"}}';

export const TEMPLATE_VARS = ["{{code}}", "{{name}}", "{{link}}", "{{expiry}}"];

export const TEMPLATES: Template[] = [
  {
    id: "tpl_otp_email",
    name: "OTP email",
    channel: "email",
    locale: "en",
    version: 4,
    status: "active",
    updated: "2h ago",
    subject: "Your worklane code: {{code}}",
    body: OTP_BODY,
    versions: [
      { v: 4, when: "2h ago", by: "khanh", note: "Shortened expiry copy to 5 minutes" },
      { v: 3, when: "6d ago", by: "khanh", note: "Added {{link}} to notification settings" },
      { v: 2, when: "22d ago", by: "mai", note: "Reworded do-not-request line" },
      { v: 1, when: "40d ago", by: "mai", note: "Initial version" },
    ],
    sends: [
      { id: "req_9f3a2c11", recipient: "d***@gmail.com", state: "verified", when: "just now" },
      { id: "req_7b1d84ec", recipient: "m***@outlook.com", state: "sent", when: "2m ago" },
      { id: "req_04ffab73", recipient: "k***@worklane.io", state: "failed", when: "9m ago" },
    ],
  },
  {
    id: "tpl_otp_sms",
    name: "OTP SMS",
    channel: "sms",
    locale: "en",
    version: 2,
    status: "active",
    updated: "1d ago",
    subject: "-",
    body: "worklane: {{code}} is your code. Expires in 5 min.",
    versions: [
      { v: 2, when: "1d ago", by: "khanh", note: "Trimmed to fit one SMS segment" },
      { v: 1, when: "18d ago", by: "khanh", note: "Initial version" },
    ],
    sends: [
      { id: "req_2c9a51de", recipient: "+84 9** *** 210", state: "verified", when: "5m ago" },
    ],
  },
  {
    id: "tpl_otp_email_vi",
    name: "OTP email",
    channel: "email",
    locale: "vi",
    version: 1,
    status: "active",
    updated: "3d ago",
    subject: "Mã xác thực worklane: {{code}}",
    body: "Mã xác thực worklane của bạn là {{code}}. Mã hết hạn sau 5 phút.",
    versions: [{ v: 1, when: "3d ago", by: "mai", note: "Bản dịch tiếng Việt" }],
    sends: [
      { id: "req_a71f0c93", recipient: "s***@proton.me", state: "verified", when: "1h ago" },
    ],
  },
  {
    id: "tpl_welcome",
    name: "Welcome",
    channel: "email",
    locale: "en",
    version: 1,
    status: "expired",
    updated: "12d ago",
    subject: "Welcome to worklane, {{name}}",
    body: WELCOME_BODY,
    versions: [{ v: 1, when: "12d ago", by: "leo", note: "Initial version" }],
    sends: [],
  },
];
