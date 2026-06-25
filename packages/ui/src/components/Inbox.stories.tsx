import type { Meta, StoryObj } from "@storybook/react-vite";
import { Inbox } from "./Inbox";

const meta: Meta<typeof Inbox> = {
  title: "Overlays/Inbox",
  component: Inbox,
  args: {
    placeholder: "Type a message...",
  },
  decorators: [
    (Story) => (
      <div style={{ width: "420px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Inbox>;

export const Default: Story = {};

export const WithValue: Story = {
  args: {
    defaultValue: "Hello world",
  },
};

export const Disabled: Story = {
  args: {
    disabled: true,
    defaultValue: "Disabled input",
  },
};

export const Empty: Story = {
  args: {
    placeholder: "No messages yet",
  },
};
