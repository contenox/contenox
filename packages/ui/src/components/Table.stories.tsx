import type { Meta, StoryObj } from "@storybook/react-vite";
import { Table, TableRow, TableCell } from "./Table";

const meta: Meta<typeof Table> = {
  title: "Data/Table",
  component: Table,
};

export default meta;
type Story = StoryObj<typeof Table>;

const sampleRows = [
  { id: "abc-001", name: "ingest-pipeline", status: "Running", owner: "alex@example.com" },
  { id: "abc-002", name: "embed-worker", status: "Idle", owner: "lisa@example.com" },
  { id: "abc-003", name: "retriever", status: "Running", owner: "alex@example.com" },
  { id: "abc-004", name: "indexer", status: "Failed", owner: "ops@example.com" },
  { id: "abc-005", name: "summarizer", status: "Running", owner: "jin@example.com" },
];

export const Default: Story = {
  render: () => (
    <Table columns={["ID", "Name", "Status", "Owner"]}>
      {sampleRows.map((row) => (
        <TableRow key={row.id}>
          <TableCell>{row.id}</TableCell>
          <TableCell>{row.name}</TableCell>
          <TableCell>{row.status}</TableCell>
          <TableCell>{row.owner}</TableCell>
        </TableRow>
      ))}
    </Table>
  ),
};

export const Empty: Story = {
  render: () => (
    <Table columns={["ID", "Name", "Status", "Owner"]}>
      <TableRow>
        <TableCell colSpan={4} style={{ textAlign: "center", opacity: 0.6 }}>
          No records
        </TableCell>
      </TableRow>
    </Table>
  ),
};

const manyRows = Array.from({ length: 24 }).map((_, i) => ({
  id: `task-${String(i + 1).padStart(4, "0")}`,
  name: `worker-${i + 1}`,
  status: ["Running", "Idle", "Failed", "Queued"][i % 4],
  owner: ["alex", "lisa", "jin", "ops"][i % 4] + "@example.com",
  latency: `${(Math.random() * 200).toFixed(1)} ms`,
}));

export const Rich: Story = {
  render: () => (
    <Table columns={["ID", "Name", "Status", "Owner", "Latency"]}>
      {manyRows.map((row) => (
        <TableRow key={row.id}>
          <TableCell>{row.id}</TableCell>
          <TableCell>{row.name}</TableCell>
          <TableCell>{row.status}</TableCell>
          <TableCell>{row.owner}</TableCell>
          <TableCell>{row.latency}</TableCell>
        </TableRow>
      ))}
    </Table>
  ),
};
