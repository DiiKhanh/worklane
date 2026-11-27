/** Centralized query keys so cache reads/writes stay consistent. */
export const qk = {
  apiKeys: ["api-keys"] as const,
  requests: ["requests"] as const,
  logs: ["logs"] as const,
  overview: ["overview"] as const,
  templates: ["templates"] as const,
  template: (id: string) => ["templates", id] as const,
  // Link data is partitioned by tenant so one tenant's cached links can never
  // render for another, even when an old response lands after a re-login.
  links: (tenant: string) => ["links", tenant] as const,
  linkPages: (tenant: string) => ["links", tenant, "page"] as const,
  linkPage: (tenant: string, cursor: string | undefined) =>
    ["links", tenant, "page", cursor ?? ""] as const,
  link: (tenant: string, code: string) => ["links", tenant, "detail", code] as const,
  linkAnalytics: (tenant: string, code: string) =>
    ["links", tenant, "analytics", code] as const,
};
