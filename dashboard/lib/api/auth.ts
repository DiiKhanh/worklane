const BASE = process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost";

export type AuthUser = { id: string; email: string; tenantId: string };
export type LoginResult = { token: string; expiresAt: string; user: AuthUser };

export async function login(email: string, password: string): Promise<LoginResult> {
  const res = await fetch(`${BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    throw new Error(res.status === 401 ? "Invalid email or password" : `Login failed (${res.status})`);
  }
  const body = await res.json();
  return {
    token: body.token,
    expiresAt: body.expires_at,
    user: { id: body.user.id, email: body.user.email, tenantId: body.user.tenant_id },
  };
}

export async function me(token: string): Promise<AuthUser> {
  const res = await fetch(`${BASE}/auth/me`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (!res.ok) throw new Error(`me failed (${res.status})`);
  const u = await res.json();
  return { id: u.id, email: u.email, tenantId: u.tenant_id };
}
