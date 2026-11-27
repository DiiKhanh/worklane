/**
 * An HTTP failure from the live API. `code` carries the server's `{"error"}` string
 * (e.g. `LINK_DISABLED`) so screens can branch on it. The message keeps the status
 * number because the query cache detects expired sessions by matching "401".
 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    readonly retryAfterSeconds?: number,
  ) {
    super(`request failed: ${status}${code ? ` ${code}` : ""}`);
    this.name = "ApiError";
  }
}

export const LINK_DISABLED = "LINK_DISABLED";
