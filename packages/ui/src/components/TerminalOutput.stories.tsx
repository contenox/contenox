import type { Meta, StoryObj } from "@storybook/react-vite";
import { TerminalOutput } from "./TerminalOutput";

const meta: Meta<typeof TerminalOutput> = {
  title: "Data/TerminalOutput",
  component: TerminalOutput,
};

export default meta;
type Story = StoryObj<typeof TerminalOutput>;

const buildLines = [
  "$ npm run build",
  "> contenox-ui@0.18.0 build",
  "> vite build",
  "",
  "vite v5.4.0 building for production...",
  "transforming...",
  "[32m✓[0m 142 modules transformed.",
  "rendering chunks...",
  "computing gzip size...",
  "dist/index.html                 0.45 kB",
  "dist/assets/index-9f3a.css     12.1 kB",
  "dist/assets/index-c2b7.js     186.4 kB",
  "[32mbuild completed in 4.21s[0m",
];

const longLines = Array.from({ length: 60 }).map((_, i) => {
  if (i % 7 === 0) return `[34m[info][0m step ${i} started`;
  if (i % 11 === 0) return `[31m[error][0m retry ${i} backoff 250ms`;
  if (i % 5 === 0) return `[33m[warn][0m slow path on iteration ${i}`;
  return `processing item ${i}... ok`;
});

export const Default: Story = {
  render: () => (
    <div style={{ height: 320 }}>
      <TerminalOutput title="Shell - build" lines={buildLines} />
    </div>
  ),
};

export const Empty: Story = {
  render: () => (
    <div style={{ height: 240 }}>
      <TerminalOutput title="Shell - idle" lines={[]} />
    </div>
  ),
};

export const Rich: Story = {
  render: () => (
    <div style={{ height: 400 }}>
      <TerminalOutput title="Shell - long-running" lines={longLines} />
    </div>
  ),
};
