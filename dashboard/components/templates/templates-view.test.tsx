import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { TemplatesView } from "./templates-view";

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("TemplatesView", () => {
  it("shows the heading and lists templates from the data source", async () => {
    renderWithQuery(<TemplatesView />);
    expect(screen.getByText("Template studio")).toBeInTheDocument();
    // Seeded by the mock DataSource, not a local fixture import.
    await waitFor(() => expect(screen.getAllByText(/OTP email/i).length).toBeGreaterThan(0));
  });
});
