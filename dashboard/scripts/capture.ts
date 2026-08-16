/**
 * Screenshots every dashboard screen in both themes for the docs gallery.
 *
 * Run against a production server for clean output (no dev overlay):
 *   pnpm build && pnpm start -p 3100 &
 *   BASE=http://localhost:3100 pnpm capture
 *
 * `reducedMotion: "reduce"` makes motion-aware components render their final
 * state immediately, so captures are deterministic (no mid-animation frames).
 */
import { chromium } from "playwright";
import path from "node:path";
import { fileURLToPath } from "node:url";

const BASE = process.env.BASE ?? "http://localhost:3100";
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const OUT = path.resolve(__dirname, "../../docs/assets/dashboard");

const ROUTES = [
  { path: "/", name: "overview" },
  { path: "/api-keys", name: "api-keys" },
  { path: "/requests", name: "requests" },
  { path: "/logs", name: "logs" },
  { path: "/playground", name: "playground" },
];
const THEMES = ["dark", "light"] as const;

async function main() {
  const browser = await chromium.launch();

  for (const theme of THEMES) {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      deviceScaleFactor: 2,
      reducedMotion: "reduce",
    });
    // Seed the persisted theme before any page script runs.
    await context.addInitScript((t) => {
      try {
        window.localStorage.setItem("theme", t as string);
      } catch {}
    }, theme);

    const page = await context.newPage();
    for (const route of ROUTES) {
      await page.goto(BASE + route.path, { waitUntil: "networkidle" });
      // Let queries resolve and any settling finish.
      await page.waitForTimeout(1000);
      const file = path.join(OUT, `${route.name}-${theme}.png`);
      await page.screenshot({ path: file });
      console.log("captured", path.relative(process.cwd(), file));
    }
    await context.close();
  }

  await browser.close();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
