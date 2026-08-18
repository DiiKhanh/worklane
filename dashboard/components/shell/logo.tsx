import { cn } from "@/lib/utils";

/**
 * The worklane logo mark from the design system: a forward chevron with a
 * dimmed leading dot. Drawn with `currentColor` so it inherits the surrounding
 * text/tile color; `LogoMark` renders it inside the primary-filled tile used in
 * the sidebar and login, matching the favicon.
 */
export function LogoGlyph({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      role="img"
      aria-label="worklane"
      className={className}
      fill="none"
    >
      <path
        d="M13 8.5 L20.5 16 L13 23.5"
        stroke="currentColor"
        strokeWidth="3.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="8.5" cy="16" r="3.2" fill="currentColor" opacity="0.5" />
    </svg>
  );
}

export function LogoMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "flex items-center justify-center rounded-[7px] bg-primary text-primary-foreground",
        className,
      )}
    >
      <LogoGlyph className="size-[70%]" />
    </span>
  );
}
