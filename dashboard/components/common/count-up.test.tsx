import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { CountUp } from "./count-up";

describe("CountUp", () => {
  beforeEach(() => {
    // Force reduced motion: the value must render immediately, no animation.
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query.includes("reduced-motion"),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
  });

  it("renders the final value instantly under reduced motion", () => {
    render(<CountUp value={1234} />);
    expect(screen.getByText("1,234")).toBeInTheDocument();
  });

  it("applies a custom formatter", () => {
    render(<CountUp value={0.87} format={(v) => `${Math.round(v * 100)}%`} />);
    expect(screen.getByText("87%")).toBeInTheDocument();
  });
});
