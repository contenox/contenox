import type { Meta, StoryObj } from "@storybook/react-vite";
import ReactMarkdown from "react-markdown";
import {
  ChatStreamingCaret,
  ChatStreamThinkingBox,
  ChatTranscriptStreamingPlaceholder,
  chatTranscriptMarkdownComponents,
} from "./chatTranscript";

const meta: Meta = {
  title: "Chat/Transcript",
};

export default meta;
type Story = StoryObj;

export const Caret: Story = {
  render: () => (
    <span>
      Streaming output<ChatStreamingCaret />
    </span>
  ),
};

export const StreamingPlaceholder: Story = {
  render: () => (
    <ChatTranscriptStreamingPlaceholder>
      Generating response
    </ChatTranscriptStreamingPlaceholder>
  ),
};

export const ThinkingBoxShort: Story = {
  render: () => (
    <ChatStreamThinkingBox>
      Considering the user's request and breaking it into steps.
    </ChatStreamThinkingBox>
  ),
};

export const ThinkingBoxLong: Story = {
  render: () => (
    <ChatStreamThinkingBox>
      {`First, I need to understand the scope of the question.\n\nThe user is asking about how the system handles concurrent requests under load. There are several axes to consider: connection pooling, queue depth, backpressure signals, and the eviction policy.\n\nI should structure the answer around those four axes, with a short example for each.`}
    </ChatStreamThinkingBox>
  ),
};

export const MarkdownComponents: Story = {
  render: () => (
    <div style={{ maxWidth: "640px" }}>
      <ReactMarkdown components={chatTranscriptMarkdownComponents}>
        {`Inline \`code\` looks like this.

\`\`\`ts
export function add(a: number, b: number): number {
  return a + b;
}
\`\`\`

> A blockquote rendered through the transcript markdown map.`}
      </ReactMarkdown>
    </div>
  ),
};
