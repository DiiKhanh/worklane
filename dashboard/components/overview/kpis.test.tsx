import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Kpis } from "./kpis";

const mockUseOverview = vi.fn();
vi.mock("@/lib/queries/use-overview", () => ({
  useOverview: () => mockUseOverview(),
}));

afterEach(() => mockUseOverview.mockReset());

describe("Kpis", () => {
  it("shows an honest empty state when the overview query errors", () => {
    mockUseOverview.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    });
    render(<Kpis />);
    expect(screen.getByText("Metrics unavailable")).toBeInTheDocument();
  });

  it("stays on the skeleton (no error text) while loading", () => {
    mockUseOverview.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    });
    render(<Kpis />);
    expect(screen.queryByText("Metrics unavailable")).not.toBeInTheDocument();
  });
});
