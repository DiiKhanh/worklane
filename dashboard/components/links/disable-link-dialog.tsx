"use client";

import { Ban } from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api/errors";
import type { Link } from "@/lib/api/types";
import { useDisableLink } from "@/lib/queries/use-links";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function disableErrorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 404) return "This link no longer exists for your tenant.";
  if (error instanceof ApiError && error.status === 503) return "Disabling is temporarily unavailable. Try again shortly.";
  return "Could not disable the link. Try again.";
}

/** Confirms the permanent disable before anything is submitted. */
export function DisableLinkDialog({
  link,
  open,
  onClose,
}: {
  link: Link;
  open: boolean;
  onClose: () => void;
}) {
  const disable = useDisableLink(link.code);

  function close() {
    if (disable.isPending) return;
    disable.reset();
    onClose();
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Disable this link permanently?</DialogTitle>
          <DialogDescription>
            New visits to <span className="font-mono text-foreground">{link.shortUrl}</span> will
            get 410 Gone within a minute. This cannot be undone: the link cannot be re-enabled, and
            shortening the same URL again will be refused. Click history is kept.
          </DialogDescription>
        </DialogHeader>
        {disable.isError && (
          <p role="alert" className="m-0 text-xs text-[var(--state-failed)]">
            {disableErrorMessage(disable.error)}
          </p>
        )}
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={close} disabled={disable.isPending}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            size="sm"
            disabled={disable.isPending}
            onClick={() =>
              disable.mutate(undefined, {
                onSuccess: () => {
                  toast.success("Link disabled", { description: link.shortUrl });
                  disable.reset();
                  onClose();
                },
              })
            }
          >
            <Ban className="size-3.5" />
            {disable.isPending ? "Disabling…" : "Disable permanently"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
