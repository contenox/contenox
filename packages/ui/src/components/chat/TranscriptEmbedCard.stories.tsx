import type { Meta, StoryObj } from "@storybook/react-vite";
import { TranscriptEmbedCard } from "./TranscriptEmbedCard";

const meta: Meta<typeof TranscriptEmbedCard> = {
  title: "Chat/TranscriptEmbedCard",
  component: TranscriptEmbedCard,
  args: {
    title: "Plan preview",
    children: (
      <div style={{ padding: 8 }}>
        <p>Step 1: Read source file</p>
        <p>Step 2: Apply transformation</p>
        <p>Step 3: Write to target path</p>
      </div>
    ),
  },
};

export default meta;
type Story = StoryObj<typeof TranscriptEmbedCard>;

export const Collapsed: Story = {};

export const Expanded: Story = {
  args: {
    defaultOpen: true,
  },
};

export const WithHeaderRight: Story = {
  args: {
    defaultOpen: true,
    title: "Diff preview",
    headerRight: "3 files changed",
    children: (
      <pre style={{ margin: 0, padding: 8, fontSize: 12 }}>
        {"+++ b/app.ts\n--- a/app.ts\n@@\n- old\n+ new"}
      </pre>
    ),
  },
};
