import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RequestDetail } from "./request-detail";
import type { OtpRequest } from "@/lib/api/types";

const req: OtpRequest = {
  id: "req_9f3a2c11",
  recipient: "d***@gmail.com",
  channel: "email",
  state: "verified",
  createdAt: new Date().toISOString(),
};

describe("RequestDetail", () => {
  it("shows the request id, state, and lifecycle stages", () => {
    render(<RequestDetail request={req} log={undefined} onBack={vi.fn()} />);
    expect(screen.getAllByText("req_9f3a2c11").length).toBeGreaterThan(0);
    expect(screen.getByText("Requested")).toBeInTheDocument();
    expect(screen.getByText("Verified")).toBeInTheDocument();
  });
});
