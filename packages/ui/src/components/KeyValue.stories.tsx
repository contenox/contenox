import type { Meta, StoryObj } from "@storybook/react-vite";
import { KeyValue } from "./KeyValue";

const meta: Meta<typeof KeyValue> = {
  title: "Primitives/KeyValue",
  component: KeyValue,
  args: {
    label: "Status",
    value: "Active",
  },
};

export default meta;
type Story = StoryObj<typeof KeyValue>;

export const Default: Story = {};

export const LongValue: Story = {
  args: {
    label: "Description",
    value: "A long value that will be truncated when the row runs out of space.",
  },
};

export const List: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.25rem", width: "320px" }}>
      <KeyValue label="ID" value="abc-123" />
      <KeyValue label="Status" value="Running" />
      <KeyValue label="Created" value="2025-01-15" />
      <KeyValue label="Owner" value="alex@example.com" />
    </div>
  ),
};
