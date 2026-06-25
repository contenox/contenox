import type { Meta, StoryObj } from "@storybook/react-vite";
import { StatusIndicator } from "./StatusIndicator";

const meta: Meta<typeof StatusIndicator> = {
  title: "Primitives/StatusIndicator",
  component: StatusIndicator,
  argTypes: {
    status: {
      control: "select",
      options: ["planned", "in-progress", "completed", "error", "warning", "info"],
    },
    size: {
      control: "select",
      options: ["sm", "md", "lg"],
    },
    showIcon: { control: "boolean" },
    progress: { control: { type: "range", min: 0, max: 100, step: 1 } },
  },
  args: {
    status: "in-progress",
    label: "Processing",
    size: "md",
    showIcon: false,
  },
  decorators: [
    (Story) => (
      <div style={{ width: "320px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StatusIndicator>;

export const Default: Story = {};

export const Planned: Story = { args: { status: "planned", label: "Planned" } };

export const Completed: Story = { args: { status: "completed", label: "Completed", progress: 100 } };

export const ErrorStatus: Story = { args: { status: "error", label: "Failed" } };

export const Warning: Story = { args: { status: "warning", label: "Warning" } };

export const Info: Story = { args: { status: "info", label: "Info" } };

export const WithProgress: Story = {
  args: { status: "in-progress", label: "Loading", progress: 45 },
};

export const WithIcon: Story = {
  args: { status: "completed", label: "Done", showIcon: true },
};
