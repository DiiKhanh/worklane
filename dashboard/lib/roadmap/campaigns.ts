/**
 * Local fixtures for the Campaigns roadmap screen. UI-only, ported verbatim from
 * the design-system kit.
 */
export type CampaignStatusValue = "sent" | "sending" | "scheduled" | "draft";
export type Campaign = {
  id: string;
  name: string;
  kind: "marketing" | "otp";
  channel: "email" | "sms";
  audience: string;
  recipients: number;
  delivered: number;
  opened: number;
  clicked: number;
  status: CampaignStatusValue;
  when: string;
  template: string;
};

export type Audience = { id: string; label: string; size: number; note: string };
export type CampaignTemplate = {
  id: string;
  label: string;
  channel: "email" | "sms";
  subject: string;
  body: string;
};

export const AUDIENCES: Audience[] = [
  { id: "aud_verified", label: "All verified users", size: 12480, note: "email verified, not unsubscribed" },
  { id: "aud_newsletter", label: "Newsletter subscribers", size: 8241, note: "opted in to product news" },
  { id: "aud_trial", label: "Trial tenants (last 14 days)", size: 612, note: "signed up, no paid plan" },
  { id: "aud_dormant", label: "Dormant accounts (90 days)", size: 3105, note: "no OTP request in 90 days" },
  { id: "aud_csv", label: "Upload a recipient list (CSV)", size: 0, note: "one email or phone per row" },
];

export const CAMPAIGN_TEMPLATES: CampaignTemplate[] = [
  { id: "tpl_otp_email", label: "OTP email · v4", channel: "email", subject: "Your worklane code: {{code}}", body: "Your worklane verification code is {{code}}. It expires in 5 minutes." },
  { id: "tpl_otp_sms", label: "OTP SMS · v2", channel: "sms", subject: "-", body: "worklane: {{code}} is your code. Expires in 5 min." },
  { id: "tpl_product_news", label: "Product news · v7", channel: "email", subject: "{{name}}, delivery logs just got faster", body: 'Hi {{name}},\n\nDelivery logs now stream in real time, and every attempt keeps its provider response.\n\nRead the notes: {{link "https://worklane.io/changelog"}}' },
  { id: "tpl_reactivation", label: "Reactivation · v2", channel: "email", subject: "Your worklane keys are still live", body: 'Hi {{name}},\n\nYour API keys are still active. Send a test OTP whenever you are ready: {{link "https://worklane.io/playground"}}' },
];

export const CAMPAIGNS: Campaign[] = [
  { id: "cmp_8fa21c", name: "August product news", kind: "marketing", channel: "email", audience: "Newsletter subscribers", recipients: 8241, delivered: 8102, opened: 3944, clicked: 812, status: "sent", when: "2d ago", template: "Product news · v7" },
  { id: "cmp_71bd04", name: "Dormant account reactivation", kind: "marketing", channel: "email", audience: "Dormant accounts (90 days)", recipients: 3105, delivered: 2981, opened: 704, clicked: 96, status: "sent", when: "9d ago", template: "Reactivation · v2" },
  { id: "cmp_2ca9f3", name: "Trial onboarding nudge", kind: "marketing", channel: "email", audience: "Trial tenants (last 14 days)", recipients: 612, delivered: 0, opened: 0, clicked: 0, status: "scheduled", when: "in 6h", template: "Product news · v7" },
  { id: "cmp_04ff7b", name: "Bulk re-verification (support batch)", kind: "otp", channel: "email", audience: "Upload a recipient list (CSV)", recipients: 218, delivered: 214, opened: 0, clicked: 0, status: "sending", when: "now", template: "OTP email · v4" },
  { id: "cmp_9d3e18", name: "SMS fallback test", kind: "otp", channel: "sms", audience: "Upload a recipient list (CSV)", recipients: 40, delivered: 0, opened: 0, clicked: 0, status: "draft", when: "-", template: "OTP SMS · v2" },
];
