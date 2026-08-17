import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ColumnDef } from "@tanstack/react-table";
import { DataTable } from "./data-table";

type Row = { id: string; name: string };
const columns: ColumnDef<Row>[] = [{ accessorKey: "name", header: "Name" }];
const data: Row[] = [{ id: "a", name: "Alpha" }];

describe("DataTable onRowClick", () => {
  it("fires onRowClick with the row when a row is activated", async () => {
    const onRowClick = vi.fn();
    render(
      <DataTable
        columns={columns}
        data={data}
        onRowClick={onRowClick}
        rowKey={(r) => r.id}
      />,
    );
    await userEvent.click(screen.getByText("Alpha"));
    expect(onRowClick).toHaveBeenCalledWith(data[0]);
  });

  it("does not make the data row clickable when onRowClick is absent", () => {
    render(<DataTable columns={columns} data={data} rowKey={(r) => r.id} />);
    const row = screen.getByText("Alpha").closest("tr");
    expect(row?.getAttribute("role")).toBeNull();
  });
});
