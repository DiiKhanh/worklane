/**
 * Pure helpers for the Templates roadmap screen: preview substitution and
 * variable-token splitting for editor highlighting.
 */
export function splitVars(text: string): { text: string; isVar: boolean }[] {
  return text
    .split(/(\{\{[^}]+\}\})/g)
    .filter(Boolean)
    .map((t) => ({ text: t, isVar: /^\{\{[^}]+\}\}$/.test(t) }));
}

export function renderPreview(tpl: { subject: string; body: string }) {
  const sub = (s: string) =>
    s
      .replace(/\{\{code\}\}/g, "418209")
      .replace(/\{\{name\}\}/g, "Daniel")
      .replace(/\{\{\s*link[^}]*\}\}/g, "wl.link/7Xq2Ab");
  return { subject: sub(tpl.subject), body: sub(tpl.body) };
}
