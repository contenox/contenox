import type { Meta, StoryObj } from "@storybook/react-vite";
import { AddNodeButton } from "./AddNodeButton";

const meta: Meta<typeof AddNodeButton> = {
  title: "Visualization/AddNodeButton",
  component: AddNodeButton,
  args: {
    x: 30,
    y: 30,
    onClick: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof AddNodeButton>;

const SvgFrame = ({ children }: { children: React.ReactNode }) => (
  <svg width="120" height="120" viewBox="0 0 120 120">
    {children}
  </svg>
);

export const Default: Story = {
  render: (args) => (
    <SvgFrame>
      <AddNodeButton {...args} x={60} y={60} />
    </SvgFrame>
  ),
};

export const Positioned: Story = {
  render: () => (
    <svg width="240" height="120" viewBox="0 0 240 120">
      <AddNodeButton x={40} y={60} onClick={() => {}} />
      <AddNodeButton x={120} y={60} onClick={() => {}} />
      <AddNodeButton x={200} y={60} onClick={() => {}} />
    </svg>
  ),
};
