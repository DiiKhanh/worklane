export type DialCode = { code: string; label: string; iso: string };

// Curated list - sufficient for the playground, easy to extend. Vietnam is the default.
export const DIAL_CODES: DialCode[] = [
  { code: "+84", label: "Vietnam", iso: "VN" },
  { code: "+1", label: "United States", iso: "US" },
  { code: "+44", label: "United Kingdom", iso: "GB" },
  { code: "+65", label: "Singapore", iso: "SG" },
  { code: "+81", label: "Japan", iso: "JP" },
  { code: "+61", label: "Australia", iso: "AU" },
  { code: "+91", label: "India", iso: "IN" },
  { code: "+49", label: "Germany", iso: "DE" },
  { code: "+33", label: "France", iso: "FR" },
  { code: "+82", label: "South Korea", iso: "KR" },
];

const E164 = /^\+[1-9]\d{7,14}$/;

// toE164 joins a dial code and a national number into an E.164 string. It strips spaces
// and a single leading 0 (common local trunk prefix) from the national part.
export function toE164(dialCode: string, national: string): string {
  const digits = national.replace(/\D/g, "").replace(/^0+/, "");
  return `${dialCode}${digits}`;
}

export function isValidE164(value: string): boolean {
  return E164.test(value);
}

// parsePhone splits an E.164 value against the known dial codes (longest match wins so
// +1 does not shadow a longer code). Returns null if it is not valid E.164 or the dial
// code is not in the list.
export function parsePhone(value: string): { dialCode: string; national: string } | null {
  if (!isValidE164(value)) return null;
  const match = [...DIAL_CODES]
    .sort((a, b) => b.code.length - a.code.length)
    .find((d) => value.startsWith(d.code));
  if (!match) return null;
  return { dialCode: match.code, national: value.slice(match.code.length) };
}
