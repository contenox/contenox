import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef } from "react";
import { ChatThread } from "./ChatThread";
import { ChatMessage } from "./ChatMessage";
import { ChatDateSeparator } from "./ChatDateSeparator";

const meta: Meta<typeof ChatThread> = {
  title: "Chat/ChatThread",
  component: ChatThread,
};

export default meta;
type Story = StoryObj<typeof ChatThread>;

export const Default: Story = {
  render: () => {
    const containerRef = useRef<HTMLDivElement>(null);
    const endRef = useRef<HTMLDivElement>(null);
    return (
      <div style={{ height: 520, display: "flex", flexDirection: "column" }}>
        <ChatThread containerRef={containerRef} endRef={endRef}>
          <ChatDateSeparator label="Today" />
          <ChatMessage role="system" roleLabel="System" timestamp="09:00">
            Session started. Workspace alpha loaded.
          </ChatMessage>
          <ChatMessage role="user" roleLabel="User" timestamp="09:01">
            Hi, can you check the status of the build pipeline?
          </ChatMessage>
          <ChatMessage role="assistant" roleLabel="Assistant" timestamp="09:01">
            Build pipeline is green. Last deploy completed 4 minutes ago.
          </ChatMessage>
          <ChatMessage role="user" roleLabel="User" timestamp="09:02">
            Great. What was the change?
          </ChatMessage>
          <ChatMessage
            role="assistant"
            roleLabel="Assistant"
            timestamp="09:02"
            isLatest
            latestLabel="Latest"
          >
            The change was a refactor of the chain executor to support
            structured tool outputs.
          </ChatMessage>
        </ChatThread>
      </div>
    );
  },
};

export const Empty: Story = {
  render: () => {
    const containerRef = useRef<HTMLDivElement>(null);
    const endRef = useRef<HTMLDivElement>(null);
    return (
      <div style={{ height: 320, display: "flex", flexDirection: "column" }}>
        <ChatThread containerRef={containerRef} endRef={endRef}>
          <div style={{ padding: 24, opacity: 0.6 }}>No messages yet.</div>
        </ChatThread>
      </div>
    );
  },
};
