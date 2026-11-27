import { CloudOff, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * A failed read. Shown in place of the data, never as zeros, with a retry that
 * refetches only the failed query.
 */
export function Unavailable({
  title,
  description,
  onRetry,
  retrying,
}: {
  title: string;
  description: string;
  onRetry: () => void;
  retrying: boolean;
}) {
  return (
    <div
      role="alert"
      className="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border px-6 py-10 text-center"
    >
      <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <CloudOff className="size-5" />
      </span>
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
      <Button size="sm" variant="outline" onClick={onRetry} disabled={retrying}>
        <RotateCw className={retrying ? "size-3.5 animate-spin motion-reduce:animate-none" : "size-3.5"} />
        {retrying ? "Retrying…" : "Retry"}
      </Button>
    </div>
  );
}
