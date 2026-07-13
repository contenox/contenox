import type { Meta, StoryObj } from "@storybook/react-vite";
import { ExecutionTimeline } from "./ExecutionTimeline";
import type { TaskEvent, CapturedStateUnit } from "./taskTypes";

const meta: Meta<typeof ExecutionTimeline> = {
  title: "Visualization/ExecutionTimeline",
  component: ExecutionTimeline,
};

export default meta;
type Story = StoryObj<typeof ExecutionTimeline>;

const baseTime = new Date("2026-05-14T12:00:00Z").getTime();
const t = (offsetMs: number) => new Date(baseTime + offsetMs).toISOString();

const events: TaskEvent[] = [
  { kind: "chain_started", timestamp: t(0), chain_id: "rag-pipeline" },
  { kind: "step_started", timestamp: t(50), task_id: "ingest", task_handler: "compose" },
  { kind: "step_completed", timestamp: t(380), task_id: "ingest", transition: "next" },
  { kind: "step_started", timestamp: t(420), task_id: "embed", task_handler: "model_exec" },
  { kind: "step_chunk", timestamp: t(560), task_id: "embed", content: "chunk 1" },
  { kind: "step_completed", timestamp: t(1500), task_id: "embed", transition: "next" },
  { kind: "step_started", timestamp: t(1600), task_id: "retrieve", task_handler: "retriever" },
  { kind: "step_failed", timestamp: t(2100), task_id: "retrieve", error: "store unreachable" },
];

const state: CapturedStateUnit[] = [
  {
    taskID: "ingest",
    taskHandler: "compose",
    inputType: "string",
    outputType: "messages",
    transition: "next",
    duration: 380,
    error: { error: null },
  },
  {
    taskID: "embed",
    taskHandler: "model_exec",
    inputType: "messages",
    outputType: "vector",
    transition: "next",
    duration: 1080,
    error: { error: null },
  },
  {
    taskID: "retrieve",
    taskHandler: "retriever",
    inputType: "vector",
    outputType: "documents",
    transition: "done",
    duration: 500,
    error: { error: "store unreachable" },
  },
];

export const LiveEvents: Story = {
  render: () => <ExecutionTimeline events={events} />,
};

export const HistoricalState: Story = {
  render: () => <ExecutionTimeline state={state} />,
};

export const ApprovalPending: Story = {
  render: () => (
    <ExecutionTimeline
      events={[
        { kind: "chain_started", timestamp: t(0), chain_id: "approval-flow" },
        { kind: "step_started", timestamp: t(100), task_id: "review", task_handler: "approval" },
        {
          kind: "approval_requested",
          timestamp: t(200),
          task_id: "review",
          approval_id: "appr-1",
          tool_name: "deploy",
        },
      ]}
    />
  ),
};
