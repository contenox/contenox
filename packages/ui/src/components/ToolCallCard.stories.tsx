import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToolCallCard } from "./ToolCallCard";

const meta: Meta<typeof ToolCallCard> = {
  title: "Data/ToolCallCard",
  component: ToolCallCard,
  args: {
    tool: "local_shell",
    title: "Ran build command",
  },
};

export default meta;
type Story = StoryObj<typeof ToolCallCard>;

export const Default: Story = {
  args: { status: "success", duration: "1.2s" },
  render: (args) => (
    <div style={{ width: 540 }}>
      <ToolCallCard {...args} />
    </div>
  ),
};

export const Pending: Story = {
  args: { status: "pending", tool: "linear", title: "Queued: create DEV-116" },
  render: (args) => (
    <div style={{ width: 540 }}>
      <ToolCallCard {...args} />
    </div>
  ),
};

export const Running: Story = {
  args: { status: "running", tool: "webtools", title: "Fetching https://example.com/docs", duration: "0.4s" },
  render: (args) => (
    <div style={{ width: 540 }}>
      <ToolCallCard {...args} />
    </div>
  ),
};

export const Success: Story = {
  args: {
    status: "success",
    tool: "linear",
    title: "Created DEV-116: Improve cold-start latency",
    duration: "0.8s",
    href: "https://linear.app/example/issue/DEV-116",
    detail: `{
  "id": "DEV-116",
  "url": "https://linear.app/example/issue/DEV-116",
  "state": "Triage",
  "assignee": "alex@example.com"
}`,
  },
  render: (args) => (
    <div style={{ width: 540 }}>
      <ToolCallCard {...args} />
    </div>
  ),
};

export const ErrorState: Story = {
  args: {
    status: "error",
    tool: "local_shell",
    title: "Build failed: exit code 1",
    duration: "4.2s",
    detail: `npm ERR! code ELIFECYCLE
npm ERR! errno 1
npm ERR! contenox-ui@0.18.0 build: vite build
npm ERR! Exit status 1
src/components/Foo.tsx(12,7): error TS2322: Type 'string' is not assignable to type 'number'.`,
  },
  render: (args) => (
    <div style={{ width: 540 }}>
      <ToolCallCard {...args} />
    </div>
  ),
};
