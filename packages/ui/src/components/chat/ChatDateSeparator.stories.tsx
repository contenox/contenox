import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatDateSeparator } from "./ChatDateSeparator";

const meta: Meta<typeof ChatDateSeparator> = {
  title: "Chat/ChatDateSeparator",
  component: ChatDateSeparator,
  args: {
    label: "Today",
  },
};

export default meta;
type Story = StoryObj<typeof ChatDateSeparator>;

export const Default: Story = {};
