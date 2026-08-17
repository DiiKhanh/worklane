/**
 * Local fixtures for the Links roadmap screen (URL shortener + click analytics).
 * UI-only, ported verbatim from the design-system kit.
 */
export type LinkClick = { ts: string; ref: string; geo: string; device: string };
export type Link = {
  code: string;
  target: string;
  clicks: number;
  ctr: number;
  created: string;
  createdFull: string;
  series: number[]; // 14-day click series, newest last
  recent: LinkClick[];
};

export const LINK_KPIS = {
  created: 412,
  clicks: 2861,
  clickRate: 0.24,
  cacheHit: 0.97,
};

export const LINKS: Link[] = [
  {
    code: "7Xq2Ab",
    target: "https://worklane.io/settings/notifications",
    clicks: 1842,
    ctr: 0.31,
    created: "2h ago",
    createdFull: "Aug 16, 2026 · 09:12",
    series: [42, 61, 55, 78, 96, 120, 143, 132, 168, 190, 176, 205, 231, 254],
    recent: [
      { ts: "just now", ref: "email · OTP email", geo: "Ho Chi Minh City, VN", device: "iOS" },
      { ts: "1m ago", ref: "email · OTP email", geo: "Ha Noi, VN", device: "Android" },
      { ts: "3m ago", ref: "sms · OTP SMS", geo: "Singapore, SG", device: "Android" },
      { ts: "6m ago", ref: "email · Welcome", geo: "Tokyo, JP", device: "macOS" },
    ],
  },
  {
    code: "k91Rme",
    target: "https://worklane.io/verify/help",
    clicks: 634,
    ctr: 0.18,
    created: "9h ago",
    createdFull: "Aug 16, 2026 · 02:40",
    series: [8, 12, 19, 22, 31, 40, 38, 46, 51, 58, 62, 71, 66, 60],
    recent: [
      { ts: "12m ago", ref: "email · OTP email", geo: "Da Nang, VN", device: "iOS" },
      { ts: "40m ago", ref: "email · OTP email", geo: "Osaka, JP", device: "Windows" },
    ],
  },
  {
    code: "Zt4bQ0",
    target: "https://acme.co/welcome",
    clicks: 297,
    ctr: 0.11,
    created: "1d ago",
    createdFull: "Aug 15, 2026 · 10:05",
    series: [2, 5, 9, 14, 18, 24, 29, 33, 30, 27, 22, 19, 15, 12],
    recent: [
      { ts: "2h ago", ref: "email · Welcome", geo: "Bangkok, TH", device: "Android" },
    ],
  },
  {
    code: "pL8vNc",
    target: "https://worklane.io/status",
    clicks: 88,
    ctr: 0.04,
    created: "3d ago",
    createdFull: "Aug 13, 2026 · 16:22",
    series: [1, 2, 4, 6, 8, 11, 9, 7, 6, 8, 10, 5, 3, 2],
    recent: [
      { ts: "5h ago", ref: "email · OTP email", geo: "Kuala Lumpur, MY", device: "iOS" },
    ],
  },
];
