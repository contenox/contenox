import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChatMessage } from "./ChatMessage";

const meta: Meta<typeof ChatMessage> = {
  title: "Chat/ChatMessage",
  component: ChatMessage,
  args: {
    role: "user",
    roleLabel: "User",
    timestamp: "12:42",
    children: "Hello, can you help me write a release note?",
  },
};

export default meta;
type Story = StoryObj<typeof ChatMessage>;

export const UserMessage: Story = {
  args: {
    role: "user",
    roleLabel: "User",
    children: "Summarize the latest engineering metrics for me.",
    copyText: "Summarize the latest engineering metrics for me.",
  },
};

export const AssistantMessage: Story = {
  args: {
    role: "assistant",
    roleLabel: "Assistant",
    children: (
      <div>
        <p>Here is a summary of the metrics:</p>
        <ul>
          <li>Deploys per week: 14</li>
          <li>Mean lead time: 3.2 days</li>
          <li>Change failure rate: 4 percent</li>
        </ul>
      </div>
    ),
    copyText: "metrics summary",
  },
};

export const SystemMessage: Story = {
  args: {
    role: "system",
    roleLabel: "System",
    children: "Session started. Context loaded from workspace alpha.",
  },
};

export const ToolMessage: Story = {
  args: {
    role: "tool",
    roleLabel: "Tool",
    children: "tool_call: local_fs.read_file path=/etc/config.yaml",
  },
};

export const Latest: Story = {
  args: {
    role: "assistant",
    roleLabel: "Assistant",
    isLatest: true,
    latestLabel: "Latest",
    children: "This is the most recent reply in the conversation.",
  },
};

export const WithError: Story = {
  args: {
    role: "assistant",
    roleLabel: "Assistant",
    children: "Partial response before the failure.",
    error: "Stream disconnected after 1.2s",
    retryLabel: "Retry",
    onRetry: () => {},
  },
};

export const Transcript: Story = {
  args: {
    role: "assistant",
    roleLabel: "Assistant",
    appearance: "transcript",
    timestamp: "2026-05-14 12:42",
    children: (
      <div>
        <p>Transcript appearance uses the full-width workbench layout.</p>
        <p>Useful for reviewing long-form responses inline with task output.</p>
      </div>
    ),
    copyText: "transcript content",
  },
};
