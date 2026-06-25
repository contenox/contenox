import type { Meta, StoryObj } from "@storybook/react-vite";
import { DiffView, type DiffLine } from "./DiffView";

const meta: Meta<typeof DiffView> = {
  title: "Data/DiffView",
  component: DiffView,
};

export default meta;
type Story = StoryObj<typeof DiffView>;

const mixedLines: DiffLine[] = [
  { type: "context", content: "import { useEffect, useState } from 'react';", oldLineNumber: 1, newLineNumber: 1 },
  { type: "context", content: "", oldLineNumber: 2, newLineNumber: 2 },
  { type: "context", content: "export function Counter() {", oldLineNumber: 3, newLineNumber: 3 },
  { type: "remove", content: "  const [count, setCount] = useState(0);", oldLineNumber: 4 },
  { type: "add", content: "  const [count, setCount] = useState<number>(0);", newLineNumber: 4 },
  { type: "add", content: "  const [label, setLabel] = useState<string>('clicks');", newLineNumber: 5 },
  { type: "context", content: "", oldLineNumber: 5, newLineNumber: 6 },
  { type: "context", content: "  useEffect(() => {", oldLineNumber: 6, newLineNumber: 7 },
  { type: "remove", content: "    document.title = `count: ${count}`;", oldLineNumber: 7 },
  { type: "add", content: "    document.title = `${label}: ${count}`;", newLineNumber: 8 },
  { type: "context", content: "  }, [count]);", oldLineNumber: 8, newLineNumber: 9 },
  { type: "context", content: "}", oldLineNumber: 9, newLineNumber: 10 },
];

const addsOnly: DiffLine[] = [
  { type: "context", content: "export const config = {", oldLineNumber: 1, newLineNumber: 1 },
  { type: "context", content: "  retries: 3,", oldLineNumber: 2, newLineNumber: 2 },
  { type: "add", content: "  backoff: 'exponential',", newLineNumber: 3 },
  { type: "add", content: "  timeoutMs: 5000,", newLineNumber: 4 },
  { type: "add", content: "  jitter: true,", newLineNumber: 5 },
  { type: "context", content: "};", oldLineNumber: 3, newLineNumber: 6 },
];

const removesOnly: DiffLine[] = [
  { type: "context", content: "export const legacy = {", oldLineNumber: 1, newLineNumber: 1 },
  { type: "remove", content: "  deprecatedFlag: true,", oldLineNumber: 2 },
  { type: "remove", content: "  oldEndpoint: '/v1/legacy',", oldLineNumber: 3 },
  { type: "remove", content: "  retries: 99,", oldLineNumber: 4 },
  { type: "context", content: "};", oldLineNumber: 5, newLineNumber: 2 },
];

export const Default: Story = {
  render: () => (
    <DiffView filePath="src/components/Counter.tsx" language="tsx" lines={mixedLines} />
  ),
};

export const AddsOnly: Story = {
  render: () => (
    <DiffView filePath="src/config/retry.ts" language="ts" lines={addsOnly} />
  ),
};

export const RemovesOnly: Story = {
  render: () => (
    <DiffView filePath="src/config/legacy.ts" language="ts" lines={removesOnly} />
  ),
};

export const Mixed: Story = {
  render: () => (
    <DiffView filePath="src/components/Counter.tsx" language="tsx" lines={mixedLines} />
  ),
};

export const Empty: Story = {
  render: () => <DiffView filePath="src/empty.ts" lines={[]} />,
};
