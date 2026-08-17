/**
 * Pure helpers for the campaign composer's live estimates. UI-only; the CSV
 * "size" is a fixed sample (218 rows) since there is no real upload/parse.
 */
export function recipientScope(a: {
  isCsv: boolean;
  size: number;
  csvLoaded: boolean;
}): number {
  return a.isCsv ? (a.csvLoaded ? 218 : 0) : a.size;
}

export function sendWindowMinutes(size: number, rate: number): number {
  if (!size || !rate) return 0;
  return Math.max(1, Math.round(size / rate));
}
