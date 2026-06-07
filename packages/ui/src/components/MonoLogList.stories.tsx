import type { Meta, StoryObj } from "@storybook/react-vite";
import { MonoLogList, MonoLogListItem } from "./MonoLogList";

const meta: Meta<typeof MonoLogList> = {
  title: "Data/MonoLogList",
  component: MonoLogList,
};

export default meta;
type Story = StoryObj<typeof MonoLogList>;

const lines = [
  "[12:01:03] tracker connected",
  "[12:01:04] chain compile started",
  "[12:01:04] resolving 4 task nodes",
  "[12:01:05] task 'fetch' -> ok (134ms)",
  "[12:01:05] task 'parse' -> ok (22ms)",
  "[12:01:06] task 'summarize' -> ok (1.21s)",
  "[12:01:07] task 'embed' -> ok (412ms)",
  "[12:01:07] chain complete",
  "[12:01:07] flushing artifacts",
  "[12:01:08] done",
];

const richLines = Array.from({ length: 40 }).map((_, i) => {
  const ts = new Date(Date.UTC(2026, 4, 14, 12, 0, i)).toISOString().slice(11, 19);
  const kinds = ["info", "warn", "trace", "info", "info", "error"];
  const kind = kinds[i % kinds.length];
  return `[${ts}] ${kind.padEnd(5)} step-${String(i).padStart(3, "0")} payload=${JSON.stringify({ n: i, ok: i % 7 !== 0 })}`;
});

export const Default: Story = {
  render: () => (
    <div style={{ width: 480 }}>
      <MonoLogList>
        {lines.map((line, i) => (
          <MonoLogListItem key={i}>{line}</MonoLogListItem>
        ))}
      </MonoLogList>
    </div>
  ),
};

export const Empty: Story = {
  render: () => (
    <div style={{ width: 480 }}>
      <MonoLogList />
    </div>
  ),
};

export const Rich: Story = {
  render: () => (
    <div style={{ width: 640 }}>
      <MonoLogList maxHeightClassName="max-h-80">
        {richLines.map((line, i) => (
          <MonoLogListItem key={i}>{line}</MonoLogListItem>
        ))}
      </MonoLogList>
    </div>
  ),
};
