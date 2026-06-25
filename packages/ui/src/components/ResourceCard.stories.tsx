import type { Meta, StoryObj } from "@storybook/react-vite";
import { ResourceCard } from "./ResourceCard";
import { KeyValue } from "./KeyValue";

const meta: Meta<typeof ResourceCard> = {
  title: "Data/ResourceCard",
  component: ResourceCard,
  args: {
    title: "ingest-pipeline",
    subtitle: "Worker that pulls documents from S3 and forwards them to the embed queue.",
    status: "default",
  },
};

export default meta;
type Story = StoryObj<typeof ResourceCard>;

export const Default: Story = {
  render: (args) => (
    <div style={{ width: 480 }}>
      <ResourceCard {...args}>
        <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <KeyValue label="ID" value="res-abc-001" />
          <KeyValue label="Created" value="2026-04-22" />
          <KeyValue label="Owner" value="alex@example.com" />
        </div>
      </ResourceCard>
    </div>
  ),
};

export const Success: Story = {
  args: { status: "success", title: "embed-worker" },
  render: (args) => (
    <div style={{ width: 480 }}>
      <ResourceCard {...args}>
        <KeyValue label="Status" value="Healthy" />
        <KeyValue label="Last run" value="2 minutes ago" />
      </ResourceCard>
    </div>
  ),
};

export const Error: Story = {
  args: { status: "error", title: "retriever", subtitle: "Failed last health check" },
  render: (args) => (
    <div style={{ width: 480 }}>
      <ResourceCard
        {...args}
        actions={{
          edit: () => undefined,
          delete: () => undefined,
        }}
      >
        <KeyValue label="Error" value="connection refused (vector-store:6333)" />
        <KeyValue label="Retries" value="3 of 5" />
      </ResourceCard>
    </div>
  ),
};

export const Warning: Story = {
  args: { status: "warning", title: "indexer", subtitle: "Catching up on backlog" },
  render: (args) => (
    <div style={{ width: 480 }}>
      <ResourceCard {...args}>
        <KeyValue label="Backlog" value="14,302 documents" />
        <KeyValue label="Throughput" value="120 docs/s" />
      </ResourceCard>
    </div>
  ),
};

export const Rich: Story = {
  args: {
    status: "success",
    title: "summarizer",
    subtitle:
      "Long-running task that consumes embed-worker output, batches documents by source, and dispatches summarization to the configured LLM provider with retries and structured output validation.",
  },
  render: (args) => (
    <div style={{ width: 540 }}>
      <ResourceCard
        {...args}
        actions={{
          edit: () => undefined,
          delete: () => undefined,
        }}
      >
        <KeyValue label="ID" value="res-summ-7f3a-9b21-4c0e" />
        <KeyValue label="Provider" value="anthropic / claude-opus-4-7" />
        <KeyValue label="Throughput" value="42 documents per second (rolling avg)" />
        <KeyValue label="Uptime" value="14 days 6 hours 22 minutes" />
        <KeyValue label="Owner" value="alex.ertli@advancedsolution.de" />
      </ResourceCard>
    </div>
  ),
};
