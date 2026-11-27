import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import { setTimeout } from "node:timers/promises";
import { chromium, type Page } from "playwright";

const dashboard = localOrigin("LINK_E2E_DASHBOARD");
const gateway = localOrigin("E2E_API_BASE");
const password = required("LINK_E2E_PASSWORD");
const destination = `https://example.com/browser/${randomUUID()}?b=2&a=1#section`;

function required(name: string): string {
  const value = process.env[name];
  assert(value, `${name} is required`);
  return value;
}

function localOrigin(name: string): string {
  const url = new URL(required(name));
  assert(url.protocol === "http:" && ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname),
    `${name} must select a local test stack`);
  return url.origin;
}

async function login(page: Page, email: string) {
  await page.goto(`${dashboard}/login`);
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL(`${dashboard}/`);
  await page.getByRole("link", { name: "Links", exact: true }).click();
  await page.getByText("No links yet", { exact: true }).waitFor();
}

function waitForAPI(page: Page, method: string, path: string, timeout = 5_000) {
  return page.waitForResponse(
    (response) => response.url() === gateway + path && response.request().method() === method,
    { timeout },
  );
}

async function create(page: Page) {
  await page.getByLabel("Destination URL").fill(destination);
  const [response] = await Promise.all([
    waitForAPI(page, "POST", "/v1/links"),
    page.getByRole("button", { name: "Create", exact: true }).click(),
  ]);
  return response;
}

async function verify() {
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({
      permissions: ["clipboard-read", "clipboard-write"],
    });
    const page = await context.newPage();
    page.setDefaultTimeout(10_000);
    await login(page, required("LINK_E2E_EMAIL"));

    const created = await create(page);
    assert.equal(created.status(), 201);
    const { short_url: shortURL, code } = await created.json();
    assert.equal(shortURL, `${gateway}/${code}`);
    await page.getByRole("button", { name: "Copy link", exact: true }).click();
    await page.getByText("Copied", { exact: true }).waitFor();
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), shortURL);

    const head = await context.request.head(shortURL, { maxRedirects: 0 });
    assert.equal(head.status(), 302);
    assert.equal(head.headers().location, destination);
    assert.equal((await head.body()).length, 0);

    const analyticsPath = `/v1/links/${code}/analytics`;
    const [initial] = await Promise.all([
      waitForAPI(page, "GET", analyticsPath),
      page.getByRole("button", { name: "View details" }).click(),
    ]);
    assert.equal(initial.status(), 200);
    assert.equal((await initial.json()).total_clicks, 0);
    await page.getByText(/No visits recorded yet/).waitFor();

    const clickAt = Date.now();
    const redirect = await context.request.get(shortURL, {
      maxRedirects: 0,
      timeout: 5_000,
      headers: { Referer: "https://referrer.example/path?private=value" },
    });
    assert.equal(redirect.status(), 302);
    assert.equal(redirect.headers().location, destination);
    assert.equal(redirect.headers()["cache-control"], "no-store");
    const deadline = clickAt + 10_000;
    let visible = false;
    while (Date.now() < deadline) {
      const remaining = deadline - Date.now();
      const [analytics] = await Promise.all([
        waitForAPI(page, "GET", analyticsPath, remaining),
        page.getByRole("button", { name: "Refresh", exact: true }).click({ timeout: remaining }),
      ]);
      assert.equal(analytics.status(), 200);
      if ((await analytics.json()).total_clicks === 1) {
        await page.getByRole("cell", { name: "https://referrer.example" })
          .waitFor({ timeout: Math.max(1, deadline - Date.now()) });
        visible = Date.now() <= deadline;
        break;
      }
      await setTimeout(150);
    }
    assert(visible, "one GET click must appear in the dashboard within 10 seconds");
    console.log(`Dashboard click visible after ${Date.now() - clickAt} ms`);

    await page.getByRole("button", { name: "Disable", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Disable this link permanently?" });
    const [disabled] = await Promise.all([
      waitForAPI(page, "POST", `/v1/links/${code}/disable`),
      dialog.getByRole("button", { name: "Disable permanently" }).click(),
    ]);
    assert.equal(disabled.status(), 200);
    assert.equal((await disabled.json()).state, "disabled");
    const disabledAt = Date.now();
    let status: number;
    do {
      const response = await context.request.head(shortURL, { maxRedirects: 0, timeout: 5_000 });
      status = response.status();
      if (status === 410) break;
      assert.equal(status, 302);
      await setTimeout(200);
    } while (Date.now() - disabledAt < 60_000);
    const gone = await context.request.get(shortURL, { maxRedirects: 0, timeout: 5_000 });
    assert.equal(gone.status(), 410);
    assert(Date.now() - disabledAt <= 60_000, "GET must return 410 within 60 seconds");

    await page.getByText(/visits get 410 Gone/).waitFor();
    await page.getByRole("button", { name: "Links", exact: true }).click();
    const conflict = await create(page);
    assert.equal(conflict.status(), 409);
    assert.deepEqual(await conflict.json(), { error: "LINK_DISABLED" });
    await page.getByRole("alert").filter({ hasText: /permanently disabled/ }).waitFor();

    await login(page, required("LINK_E2E_SECOND_EMAIL"));
    assert.equal(await page.getByText(shortURL, { exact: true }).count(), 0);
  } finally {
    await browser.close();
  }
}

verify().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
