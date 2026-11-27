/**
 * Reads the tenant id claim from a JWT without verifying it. The result only
 * partitions the client-side query cache; the API authorizes every request from
 * the verified token, so a forged claim can never reach another tenant's data.
 * Returns "" when the token is absent or malformed.
 */
export function tenantFromToken(token: string): string {
  const payload = token.split(".")[1];
  if (!payload) return "";
  try {
    const json = atob(payload.replace(/-/g, "+").replace(/_/g, "/"));
    const claims = JSON.parse(json) as { tenant_id?: unknown };
    return typeof claims.tenant_id === "string" ? claims.tenant_id : "";
  } catch {
    return "";
  }
}
