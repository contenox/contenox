import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatProcessingBar } from "./ChatProcessingBar";

const meta: Meta<typeof ChatProcessingBar> = {
  title: "Chat/ChatProcessingBar",
  component: ChatProcessingBar,
  args: {
    label: "Assistant is thinking...",
  },
};

export default meta;
type Story = StoryObj<typeof ChatProcessingBar>;

export const Default: Story = {};

export const WithStop: Story = {
  args: {
    label: "Running chain step 2 of 4",
    onStop: () => {},
    stopLabel: "Stop",
  },
};
