import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatScrollToLatest } from "./ChatScrollToLatest";

const meta: Meta<typeof ChatScrollToLatest> = {
  title: "Chat/ChatScrollToLatest",
  component: ChatScrollToLatest,
  args: {
    visible: true,
    label: "Jump to latest",
    onClick: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof ChatScrollToLatest>;

export const Default: Story = {
  render: (args) => (
    <div style={{ position: "relative", height: 200, border: "1px dashed #888" }}>
      <ChatScrollToLatest {...args} />
    </div>
  ),
};
