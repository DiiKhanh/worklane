import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TemplatesView } from "./templates-view";

describe("TemplatesView", () => {
  it("renders the roadmap note and the template list heading", () => {
    render(<TemplatesView />);
    expect(screen.getByText(/Roadmap screen/)).toBeInTheDocument();
    expect(screen.getByText("Template studio")).toBeInTheDocument();
  });
});
