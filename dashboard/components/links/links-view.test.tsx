import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { LinksView } from "./links-view";

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("LinksView", () => {
  it("lists links from the data source, not a roadmap fixture", async () => {
    renderWithQuery(<LinksView />);
    expect(screen.getByRole("heading", { name: "Links" })).toBeInTheDocument();
    expect(screen.queryByText(/Roadmap screen/)).not.toBeInTheDocument();
    // Seeded by the mock DataSource.
    await waitFor(() => expect(screen.getByText("wl.link/7Xq2Ab")).toBeInTheDocument());
  });

  it("creates a short link and shows it in the list", async () => {
    const user = userEvent.setup();
    renderWithQuery(<LinksView />);
    await user.type(screen.getByLabelText("Long URL"), "https://example.com/pricing");
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(screen.getByText(/Short link ready/)).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("https://example.com/pricing")).toBeInTheDocument());
    expect(screen.getByLabelText("Long URL")).toHaveValue("");
  });

  it("surfaces a rejected URL instead of creating a link", async () => {
    const user = userEvent.setup();
    renderWithQuery(<LinksView />);
    await user.type(screen.getByLabelText("Long URL"), "javascript:alert(1)");
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/invalid url/));
  });

  it("opens a link's detail with its click analytics", async () => {
    const user = userEvent.setup();
    renderWithQuery(<LinksView />);
    await user.click(await screen.findByText("wl.link/k91Rme"));
    await waitFor(() => expect(screen.getByText("Recent clicks")).toBeInTheDocument());
    expect(screen.getByText("https://worklane.io/verify/help")).toBeInTheDocument();
    expect(screen.getByText("Total clicks")).toBeInTheDocument();
  });
});
