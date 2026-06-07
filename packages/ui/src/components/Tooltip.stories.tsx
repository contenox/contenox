import type { Meta, StoryObj } from "@storybook/react-vite";
import { Tooltip } from "./Tooltip";
import { Button } from "./Button";

const meta: Meta<typeof Tooltip> = {
  title: "Overlays/Tooltip",
  component: Tooltip,
  argTypes: {
    position: {
      control: "select",
      options: ["top", "bottom", "left", "right"],
    },
  },
  args: {
    content: "Tooltip text",
    position: "top",
    children: <Button>Hover me</Button>,
  },
  decorators: [
    (Story) => (
      <div style={{ padding: "5rem", display: "flex", justifyContent: "center" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Tooltip>;

export const Default: Story = {};

export const Top: Story = {
  args: { position: "top", content: "Tooltip on top" },
};

export const Bottom: Story = {
  args: { position: "bottom", content: "Tooltip on bottom" },
};

export const Left: Story = {
  args: { position: "left", content: "Tooltip on left" },
};

export const Right: Story = {
  args: { position: "right", content: "Tooltip on right" },
};

export const AllPositions: Story = {
  render: () => (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(2, 1fr)",
        gap: "4rem",
        padding: "4rem",
      }}
    >
      <Tooltip content="Top tooltip" position="top">
        <Button>Top</Button>
      </Tooltip>
      <Tooltip content="Bottom tooltip" position="bottom">
        <Button>Bottom</Button>
      </Tooltip>
      <Tooltip content="Left tooltip" position="left">
        <Button>Left</Button>
      </Tooltip>
      <Tooltip content="Right tooltip" position="right">
        <Button>Right</Button>
      </Tooltip>
    </div>
  ),
};
