import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatThreadSkeleton } from "./ChatThreadSkeleton";

const meta: Meta<typeof ChatThreadSkeleton> = {
  title: "Chat/ChatThreadSkeleton",
  component: ChatThreadSkeleton,
  args: {
    rows: 5,
  },
};

export default meta;
type Story = StoryObj<typeof ChatThreadSkeleton>;

export const Default: Story = {};

export const ShortList: Story = {
  args: { rows: 2 },
};

export const LongList: Story = {
  args: { rows: 8 },
};
