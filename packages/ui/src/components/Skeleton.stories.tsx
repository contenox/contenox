import type { Meta, StoryObj } from "@storybook/react-vite";
import { Skeleton } from "./Skeleton";

const meta: Meta<typeof Skeleton> = {
  title: "Primitives/Skeleton",
  component: Skeleton,
  argTypes: {
    variant: {
      control: "select",
      options: ["line", "circle"],
    },
  },
  args: {
    variant: "line",
  },
};

export default meta;
type Story = StoryObj<typeof Skeleton>;

export const Default: Story = {
  render: (args) => (
    <div style={{ width: "300px" }}>
      <Skeleton {...args} />
    </div>
  ),
};

export const Circle: Story = {
  args: { variant: "circle" },
};

export const TextBlock: Story = {
  render: () => (
    <div style={{ width: "300px", display: "flex", flexDirection: "column", gap: "0.5rem" }}>
      <Skeleton />
      <Skeleton style={{ width: "80%" }} />
      <Skeleton style={{ width: "60%" }} />
    </div>
  ),
};

export const Avatar: Story = {
  render: () => (
    <div style={{ display: "flex", gap: "1rem", alignItems: "center" }}>
      <Skeleton variant="circle" />
      <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: "0.5rem" }}>
        <Skeleton style={{ width: "120px" }} />
        <Skeleton style={{ width: "80px" }} />
      </div>
    </div>
  ),
};
