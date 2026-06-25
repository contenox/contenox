import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskEventFeed } from "./TaskEventFeed";
import type { TaskEvent } from "./taskTypes";

const meta: Meta<typeof TaskEventFeed> = {
  title: "Visualization/TaskEventFeed",
  component: TaskEventFeed,
};

export default meta;
type Story = StoryObj<typeof TaskEventFeed>;

const baseTime = new Date("2026-05-14T12:00:00Z").getTime();
const t = (offsetMs: number) => new Date(baseTime + offsetMs).toISOString();

const events: TaskEvent[] = [
  {
    kind: "chain_started",
    timestamp: t(0),
    request_id: "req-001",
    chain_id: "rag-pipeline",
  },
  {
    kind: "step_started",
    timestamp: t(120),
    task_id: "ingest",
    task_handler: "compose",
  },
  {
    kind: "step_completed",
    timestamp: t(380),
    task_id: "ingest",
    task_handler: "compose",
    transition: "next",
  },
  {
    kind: "step_started",
    timestamp: t(420),
    task_id: "embed",
    task_handler: "model_exec",
    model_name: "nomic-embed-text",
  },
  {
    kind: "step_chunk",
    timestamp: t(560),
    task_id: "embed",
    content: "embedding chunk 1/4",
  },
  {
    kind: "step_chunk",
    timestamp: t(720),
    task_id: "embed",
    content: "embedding chunk 2/4",
  },
  {
    kind: "step_completed",
    timestamp: t(1500),
    task_id: "embed",
    transition: "next",
  },
  {
    kind: "step_started",
    timestamp: t(1600),
    task_id: "retrieve",
    task_handler: "retriever",
  },
  {
    kind: "step_failed",
    timestamp: t(2100),
    task_id: "retrieve",
    error: "vector store unreachable",
  },
  {
    kind: "chain_failed",
    timestamp: t(2200),
    chain_id: "rag-pipeline",
    error: "retrieve step failed",
  },
];

export const Default: Story = {
  render: () => <TaskEventFeed events={events} />,
};

export const Short: Story = {
  render: () => <TaskEventFeed events={events.slice(0, 3)} />,
};

export const Truncated: Story = {
  render: () => {
    const many: TaskEvent[] = Array.from({ length: 60 }).map((_, i) => ({
      kind: i % 7 === 6 ? "step_failed" : "step_completed",
      timestamp: t(i * 200),
      task_id: `task-${i + 1}`,
      task_handler: "model_exec",
      transition: "next",
      error: i % 7 === 6 ? "transient timeout" : undefined,
    }));
    return <TaskEventFeed events={many} />;
  },
};
