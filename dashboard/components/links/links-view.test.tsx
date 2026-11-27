import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DataSource } from "@/lib/api/source";
import { MockDataSource } from "@/lib/api/mock";
import { ApiError } from "@/lib/api/errors";
import type { LinkPage } from "@/lib/api/types";
import { useUIStore } from "@/lib/store/ui";
import { LinksView } from "./links-view";

let source: DataSource;
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  getDataSource: () => source,
}));

const now = () => Date.UTC(2026, 10, 28, 12);

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <LinksView />
    </QueryClientProvider>,
  );
}

async function openRow(user: ReturnType<typeof userEvent.setup>, text: string) {
  const cell = await screen.findByText(text);
  await user.click(cell.closest("tr")!);
}

beforeEach(() => {
  source = new MockDataSource({ now });
});

afterEach(() => {
  vi.unstubAllEnvs();
  useUIStore.getState().setToken("");
});

describe("LinksView", () => {
  it("lists links with their state and no fixture-only metrics", async () => {
    renderView();
    const table = await screen.findByRole("table");
    expect(within(table).getAllByText("active")).toHaveLength(2);
    expect(within(table).getByText("disabled")).toBeInTheDocument();
    for (const fixture of [/CTR/, /Click rate/, /Cache hit/, /wl\.link/, /Roadmap screen/]) {
      expect(screen.queryByText(fixture)).not.toBeInTheDocument();
    }
  });

  it("creates a link, shows the returned short URL and copies it", async () => {
    const user = userEvent.setup();
    renderView();
    await user.type(screen.getByLabelText("Destination URL"), "https://example.com/new");
    await user.keyboard("{Enter}");

    const result = await screen.findByRole("status");
    expect(within(result).getByText("Short link created")).toBeInTheDocument();
    const shortUrl = within(result).getByRole("link").textContent!;
    expect(shortUrl).toMatch(/^https:\/\/short\.example\/\w+$/);
    // The new link is listed on the first page.
    await waitFor(() => expect(within(screen.getByRole("table")).getByText(shortUrl)).toBeInTheDocument());

    await user.click(within(result).getByRole("button", { name: "Copy link" }));
    expect(await navigator.clipboard.readText()).toBe(shortUrl);
    expect(within(result).getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("reports dedup, disabled conflicts and invalid input clearly", async () => {
    const user = userEvent.setup();
    renderView();
    const input = screen.getByLabelText("Destination URL");

    await user.type(input, "https://worklane.example/verify/help{Enter}");
    expect(await screen.findByText(/already shortened - existing link returned/)).toBeInTheDocument();

    await user.type(input, "https://worklane.example/status{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent(/permanently disabled/);
    expect(input).toHaveAttribute("aria-invalid", "true");

    await user.clear(input);
    await user.type(input, "ftp://example.com{Enter}");
    expect(screen.getByRole("alert")).toHaveTextContent(/Only http:\/\/ and https:\/\//);
  });

  it("keeps metadata and copy usable when analytics fail, without showing zeros", async () => {
    const user = userEvent.setup();
    const failing = new MockDataSource({ now });
    let analyticsDown = true;
    const real = failing.getLinkAnalytics.bind(failing);
    failing.getLinkAnalytics = (code) =>
      analyticsDown ? Promise.reject(new ApiError(503, "link: unavailable")) : real(code);
    source = failing;
    renderView();

    await openRow(user, "https://worklane.example/verify/help");
    expect(await screen.findByText("Analytics unavailable")).toBeInTheDocument();
    expect(screen.queryByText("Total clicks")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy link" })).toBeEnabled();
    expect(screen.getByRole("heading", { name: /short\.example/ })).toBeInTheDocument();

    analyticsDown = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Total clicks")).toBeInTheDocument();
    // The seeded link has 54 clicks, all inside the 14-day window.
    expect(screen.getAllByText("54")).toHaveLength(2);
  });

  it("disables only after confirmation and reflects the new state", async () => {
    const user = userEvent.setup();
    const disable = vi.spyOn(source, "disableLink");
    renderView();
    await openRow(user, "https://worklane.example/verify/help");

    await user.click(await screen.findByRole("button", { name: "Disable" }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent(/cannot be undone/);
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(disable).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Disable" }));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Disable permanently" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument());
    expect(disable).toHaveBeenCalledTimes(1);
    expect(screen.getByText(/visits get 410 Gone/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Links" }));
    const row = (await screen.findByText("https://worklane.example/verify/help")).closest("tr")!;
    await waitFor(() => expect(within(row).getByText("disabled")).toBeInTheDocument());
  });

  it("pages through the cursor list with the keyboard", async () => {
    const user = userEvent.setup();
    for (let i = 0; i < 20; i++) await source.createLink(`https://example.com/p/${i}`);
    renderView();

    await screen.findByText("https://example.com/p/19");
    expect(screen.getByText("Page 1")).toBeInTheDocument();
    screen.getByRole("button", { name: "Next" }).focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Page 2")).toBeInTheDocument();
    expect(await screen.findByText("https://worklane.example/status")).toBeInTheDocument();
    expect(screen.queryByText("https://example.com/p/19")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Previous" }));
    expect(await screen.findByText("https://example.com/p/19")).toBeInTheDocument();
  });

  it("opens a link from the keyboard and returns focus to its row", async () => {
    const user = userEvent.setup();
    renderView();
    const row = (await screen.findByText("https://worklane.example/verify/help")).closest("tr")!;
    row.focus();
    await user.keyboard("{Enter}");
    const back = await screen.findByRole("button", { name: "Links" });
    expect(back).toHaveFocus();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(row).toHaveFocus());
  });

  it("shows an unavailable state with retry when the list fails", async () => {
    const user = userEvent.setup();
    const listLinks = vi
      .spyOn(source, "listLinks")
      .mockRejectedValueOnce(new ApiError(503, "link: unavailable"));
    renderView();
    expect(await screen.findByText("Links unavailable")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("table")).toBeInTheDocument();
    expect(listLinks).toHaveBeenCalledTimes(2);
  });

  it("never shows a previous tenant's links after the session changes", async () => {
    vi.stubEnv("NEXT_PUBLIC_DATA_SOURCE", "live");
    const tokenFor = (tenant: string) => `h.${btoa(JSON.stringify({ tenant_id: tenant }))}.s`;
    let releaseB: (page: LinkPage) => void = () => {};
    const pageFor = (tenant: string): LinkPage => ({
      items: [
        {
          code: tenant,
          shortUrl: `https://short.example/${tenant}`,
          longUrl: `https://${tenant}.example/secret`,
          state: "active",
          createdAt: "2026-11-28T00:00:00Z",
          disabledAt: null,
        },
      ],
      nextCursor: null,
    });
    const tenantSource = new MockDataSource({ now });
    tenantSource.listLinks = () => {
      const tenant = JSON.parse(atob(useUIStore.getState().token.split(".")[1])).tenant_id as string;
      return tenant === "tenantA" ? Promise.resolve(pageFor(tenant)) : new Promise((r) => (releaseB = r));
    };
    source = tenantSource;

    act(() => useUIStore.getState().setToken(tokenFor("tenantA")));
    renderView();
    expect(await screen.findByText("https://tenantA.example/secret")).toBeInTheDocument();

    act(() => useUIStore.getState().setToken(tokenFor("tenantB")));
    await waitFor(() => expect(screen.queryByText("https://tenantA.example/secret")).not.toBeInTheDocument());
    expect(screen.getByLabelText("Loading links")).toBeInTheDocument();

    act(() => releaseB(pageFor("tenantB")));
    expect(await screen.findByText("https://tenantB.example/secret")).toBeInTheDocument();
    expect(screen.queryByText("https://tenantA.example/secret")).not.toBeInTheDocument();
  });
});
