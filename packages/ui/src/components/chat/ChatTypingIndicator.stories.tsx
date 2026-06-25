import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatTypingIndicator } from "./ChatTypingIndicator";

const meta: Meta<typeof ChatTypingIndicator> = {
  title: "Chat/ChatTypingIndicator",
  component: ChatTypingIndicator,
  args: {
    "aria-label": "Assistant is typing",
  },
};

export default meta;
type Story = StoryObj<typeof ChatTypingIndicator>;

export const Default: Story = {};
