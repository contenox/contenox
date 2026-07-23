import type { Meta, StoryObj } from "@storybook/react-vite";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
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
      Streaming output
      <ChatStreamingCaret />
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

const RICH_MARKDOWN = `# Heading level 1

A paragraph with **bold**, _italic_, inline \`code\`, and a [link to the docs](https://example.com).

## Lists

- First bullet
- Second bullet
  1. Nested ordered
  2. Another one

## A GFM table

| Provider | Streaming | Notes            |
| -------- | :-------: | ---------------- |
| modeld   |    yes    | local, 24/7 tier |
| openai   |    yes    | remote           |

## Syntax highlighting

\`\`\`ts
export function add(a: number, b: number): number {
  return a + b; // language: typescript
}
\`\`\`

\`\`\`python
def greet(name: str) -> str:
    return f"hello {name}"
\`\`\`

\`\`\`bash
curl -s https://example.com | jq '.data[]'
\`\`\`

> A blockquote rendered through the transcript markdown map.

---

Trailing paragraph after a rule.`;

export const MarkdownComponents: Story = {
  render: () => (
    <div style={{ maxWidth: "680px" }}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={chatTranscriptMarkdownComponents}
      >
        {RICH_MARKDOWN}
      </ReactMarkdown>
    </div>
  ),
};

/** Same content under the dark theme flip — verifies the vs2015 highlight palette. */
export const MarkdownComponentsDark: Story = {
  render: () => (
    <div
      className="dark bg-dark-surface-100 text-dark-text"
      style={{ maxWidth: "680px", padding: 16 }}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={chatTranscriptMarkdownComponents}
      >
        {RICH_MARKDOWN}
      </ReactMarkdown>
    </div>
  ),
};
