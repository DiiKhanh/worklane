"use client";

import { useState } from "react";
import { CircleCheck, Link as LinkIcon } from "lucide-react";
import { ApiError, LINK_DISABLED } from "@/lib/api/errors";
import type { CreateLinkResult } from "@/lib/api/types";
import { useCreateLink } from "@/lib/queries/use-links";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CopyLinkButton } from "./copy-link-button";

const MAX_DESTINATION_BYTES = 4096;

/**
 * Cheap checks that spare a round trip. link-svc stays authoritative for the rest
 * of the policy (localhost, internal IPs, credentials and the short host itself).
 */
function localProblem(value: string): string | null {
  if (new TextEncoder().encode(value).length > MAX_DESTINATION_BYTES) {
    return "Destination URLs are limited to 4,096 bytes.";
  }
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return "Enter an absolute URL starting with http:// or https://.";
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    return "Only http:// and https:// destinations can be shortened.";
  }
  return null;
}

function createErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) return "Could not create the link. Check your connection and try again.";
  switch (error.status) {
    case 400:
      return "This destination is not accepted. Use a public http(s) URL without credentials; local, internal and short-link addresses are rejected.";
    case 409:
      return error.code === LINK_DISABLED
        ? "This URL already has a short link that was permanently disabled. Disabled links cannot be re-enabled or replaced with a new code."
        : "This URL conflicts with an existing link.";
    case 429:
      return error.retryAfterSeconds
        ? `Link creation limit reached. Try again in ${error.retryAfterSeconds}s.`
        : "Link creation limit reached. Try again shortly.";
    case 503:
      return "Link creation is temporarily unavailable. Try again shortly.";
    default:
      return `Could not create the link (HTTP ${error.status}).`;
  }
}

export function CreateLinkForm({
  onCreated,
  onOpen,
}: {
  onCreated: () => void;
  onOpen: (code: string) => void;
}) {
  const create = useCreateLink();
  const [value, setValue] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const [result, setResult] = useState<CreateLinkResult | null>(null);
  const error = problem ?? (create.isError ? createErrorMessage(create.error) : null);

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (create.isPending) return;
    const local = localProblem(value);
    setProblem(local);
    setResult(null);
    if (local) return;
    create.mutate(value, {
      onSuccess: (res) => {
        setResult(res);
        setValue("");
        onCreated();
      },
    });
  }

  return (
    <div className="grid gap-3">
      <form onSubmit={onSubmit} noValidate className="grid gap-1.5">
        <Label htmlFor="link-destination">Destination URL</Label>
        <div className="flex gap-2">
          <Input
            id="link-destination"
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            value={value}
            onChange={(e) => {
              setValue(e.target.value);
              setProblem(null);
              if (create.isError) create.reset();
            }}
            placeholder="https://example.com/landing"
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? "link-destination-error" : undefined}
            className="font-mono text-[13px]"
          />
          <Button type="submit" className="shrink-0" disabled={!value.trim() || create.isPending}>
            <LinkIcon className="size-4" />
            {create.isPending ? "Creating…" : "Create"}
          </Button>
        </div>
        {error && (
          <p id="link-destination-error" role="alert" className="m-0 text-xs text-[var(--state-failed)]">
            {error}
          </p>
        )}
      </form>

      {result && (
        <div
          role="status"
          className="flex flex-col gap-3 rounded-lg border border-border bg-muted/40 px-3 py-2.5 sm:flex-row sm:items-center"
        >
          <CircleCheck className="hidden size-4 shrink-0 text-[var(--state-verified)] sm:block" />
          <div className="min-w-0 flex-1">
            <p className="m-0 text-xs text-muted-foreground">
              {result.created ? "Short link created" : "This URL was already shortened - existing link returned"}
            </p>
            <a
              href={result.shortUrl}
              target="_blank"
              rel="noreferrer"
              className="block truncate font-mono text-[13px] text-foreground underline-offset-4 hover:underline"
            >
              {result.shortUrl}
            </a>
          </div>
          <div className="flex shrink-0 gap-2">
            <CopyLinkButton value={result.shortUrl} />
            <Button size="sm" variant="ghost" onClick={() => onOpen(result.code)}>
              View details
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
