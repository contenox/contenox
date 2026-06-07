import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LayoutControls } from "./LayoutControls";
import type { LayoutDirection } from "./LayoutControls";

const meta: Meta<typeof LayoutControls> = {
  title: "Visualization/LayoutControls",
  component: LayoutControls,
};

export default meta;
type Story = StoryObj<typeof LayoutControls>;

export const Horizontal: Story = {
  render: () => {
    const [direction, setDirection] = useState<LayoutDirection>("horizontal");
    return <LayoutControls direction={direction} onChangeDirection={setDirection} />;
  },
};

export const Vertical: Story = {
  render: () => {
    const [direction, setDirection] = useState<LayoutDirection>("vertical");
    return <LayoutControls direction={direction} onChangeDirection={setDirection} />;
  },
};
