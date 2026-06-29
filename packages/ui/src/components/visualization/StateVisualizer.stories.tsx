import type { Meta, StoryObj } from "@storybook/react-vite";
import { StateVisualizer } from "./StateVisualizer";
import type { CapturedStateUnit } from "./taskTypes";

const meta: Meta<typeof StateVisualizer> = {
  title: "Visualization/StateVisualizer",
  component: StateVisualizer,
};

export default meta;
type Story = StoryObj<typeof StateVisualizer>;

const sampleState: CapturedStateUnit[] = [
  {
    taskID: "ingest",
    taskType: "compose",
    inputType: "string",
    outputType: "messages",
    transition: "next",
    duration: 42_000, // 42 µs (durations are nanoseconds, as sent by the backend)
    error: { error: null },
  },
  {
    taskID: "embed",
    taskType: "model_exec",
    inputType: "messages",
    outputType: "vector",
    transition: "next",
    duration: 1_380_000_000, // 1.38 s
    error: { error: null },
  },
  {
    taskID: "retrieve",
    taskType: "retriever",
    inputType: "vector",
    outputType: "documents",
    transition: "next",
    duration: 257_000_000, // 257 ms
    error: { error: null },
  },
  {
    taskID: "summarize",
    taskType: "model_exec",
    inputType: "documents",
    outputType: "string",
    transition: "done",
    duration: 3_420_000_000, // 3.42 s
    error: { error: "context window exceeded" },
  },
];

export const Default: Story = {
  render: () => <StateVisualizer state={sampleState} />,
};

export const SingleEntry: Story = {
  render: () => <StateVisualizer state={[sampleState[0]]} />,
};

export const WithErrors: Story = {
  render: () => (
    <StateVisualizer
      state={sampleState.map((u, i) => ({
        ...u,
        error: { error: i % 2 === 0 ? "transient backend failure" : null },
      }))}
    />
  ),
};
