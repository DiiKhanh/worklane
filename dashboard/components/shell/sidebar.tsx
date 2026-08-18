import Link from "next/link";
import { Nav } from "./nav";
import { LogoMark } from "./logo";
import { TenantSwitcher } from "./tenant-switcher";

export function Sidebar() {
  return (
    <aside className="hidden w-64 shrink-0 flex-col border-r border-border bg-sidebar md:flex">
      <div className="flex h-14 items-center px-5">
        <Link href="/" className="flex items-center gap-2 outline-none">
          <LogoMark className="size-6" />
          <span className="text-[15px] font-semibold tracking-tight">
            worklane
          </span>
        </Link>
      </div>

      <div className="mt-2 flex-1">
        <p className="px-5 pb-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          Platform
        </p>
        <Nav />
      </div>

      <div className="p-3">
        <TenantSwitcher />
      </div>
    </aside>
  );
}
