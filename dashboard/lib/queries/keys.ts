/** Centralized query keys so cache reads/writes stay consistent. */
export const qk = {
  apiKeys: ["api-keys"] as const,
  requests: ["requests"] as const,
  logs: ["logs"] as const,
  overview: ["overview"] as const,
  templates: ["templates"] as const,
  template: (id: string) => ["templates", id] as const,
  links: ["links"] as const,
  link: (code: string) => ["links", code] as const,
};
