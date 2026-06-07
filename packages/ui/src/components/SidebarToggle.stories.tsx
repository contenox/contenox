import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SidebarToggle } from "./SidebarToggle";

const meta: Meta<typeof SidebarToggle> = {
  title: "Primitives/SidebarToggle",
  component: SidebarToggle,
  argTypes: {
    isOpen: { control: "boolean" },
  },
  args: {
    isOpen: false,
  },
};

export default meta;
type Story = StoryObj<typeof SidebarToggle>;

export const Closed: Story = {
  args: { isOpen: false, onToggle: () => {} },
};

export const Open: Story = {
  args: { isOpen: true, onToggle: () => {} },
};

export const Interactive: Story = {
  render: () => {
    const [open, setOpen] = useState(false);
    return <SidebarToggle isOpen={open} onToggle={() => setOpen((v) => !v)} />;
  },
};
