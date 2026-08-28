import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OverviewCharts } from "./overview-charts";

const mockUseOverview = vi.fn();
vi.mock("@/lib/queries/use-overview", () => ({
  useOverview: () => mockUseOverview(),
}));

afterEach(() => mockUseOverview.mockReset());

describe("OverviewCharts", () => {
  it("shows an unavailable message in both panels when the query errors", () => {
    mockUseOverview.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    });
    render(<OverviewCharts />);
    // Panel chrome still renders; each chart body falls back to the message.
    expect(screen.getByText("Verification volume")).toBeInTheDocument();
    expect(screen.getByText("Conversion funnel")).toBeInTheDocument();
    expect(
      screen.getAllByText("Stats could not be loaded"),
    ).toHaveLength(2);
  });

  it("stays on the skeleton (no error text) while loading", () => {
    mockUseOverview.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    });
    render(<OverviewCharts />);
    expect(
      screen.queryByText("Stats could not be loaded"),
    ).not.toBeInTheDocument();
  });
});
