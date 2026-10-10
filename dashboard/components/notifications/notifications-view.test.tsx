import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { NotificationsView } from "./notifications-view";

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("NotificationsView", () => {
  it("lists notifications from the data source", async () => {
    renderWithQuery(<NotificationsView />);
    expect(screen.getByRole("heading", { name: "Notifications" })).toBeInTheDocument();
    // Seeded by the mock DataSource.
    await waitFor(() => expect(screen.getByText("ntf_81b3f2d4")).toBeInTheDocument());
    expect(screen.getByText("ntf_e07c4418")).toBeInTheDocument();
  });

  it("filters the list by state", async () => {
    const user = userEvent.setup();
    renderWithQuery(<NotificationsView />);
    await screen.findByText("ntf_81b3f2d4");
    await user.click(screen.getByRole("button", { name: "suppressed" }));
    expect(screen.getByText("ntf_e07c4418")).toBeInTheDocument();
    expect(screen.queryByText("ntf_81b3f2d4")).not.toBeInTheDocument();
  });

  it("opens a notification's detail with its engagement events", async () => {
    const user = userEvent.setup();
    renderWithQuery(<NotificationsView />);
    await user.click(await screen.findByText("ntf_81b3f2d4"));
    await waitFor(() => expect(screen.getByText("Engagement")).toBeInTheDocument());
    expect(screen.getByText("clicked")).toBeInTheDocument();
    expect(screen.getByText("re_8Zk2pQ41")).toBeInTheDocument();
  });

  it("shows the failure reason of a failed notification", async () => {
    const user = userEvent.setup();
    renderWithQuery(<NotificationsView />);
    await user.click(await screen.findByText("ntf_b64f19a3"));
    await waitFor(() => expect(screen.getByText("Failure reason")).toBeInTheDocument());
    expect(screen.getByText("smtp: 550 mailbox unavailable")).toBeInTheDocument();
  });
});
