import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  InlineAttachmentRenderer,
  InlineAttachments,
  type InlineAttachment,
} from "./InlineAttachmentRenderer";

const meta: Meta<typeof InlineAttachmentRenderer> = {
  title: "Data/InlineAttachmentRenderer",
  component: InlineAttachmentRenderer,
};

export default meta;
type Story = StoryObj<typeof InlineAttachmentRenderer>;

const fileView: InlineAttachment = {
  kind: "file_view",
  path: "src/utils/format.ts",
  text: `export function formatDuration(ms: number): string {
  if (ms < 1000) return \`\${ms}ms\`;
  const s = ms / 1000;
  if (s < 60) return \`\${s.toFixed(1)}s\`;
  const m = Math.floor(s / 60);
  return \`\${m}m \${Math.round(s % 60)}s\`;
}`,
};

const terminalExcerpt: InlineAttachment = {
  kind: "terminal_excerpt",
  command: "go test ./...",
  output: `ok  	github.com/contenox/runtime/libtracker	0.182s
ok  	github.com/contenox/runtime/taskengine	1.044s
FAIL	github.com/contenox/runtime/hookengine	0.301s
--- FAIL: TestHookDispatch (0.10s)
    hook_test.go:42: expected 3 dispatches, got 2`,
  capturedAt: "2026-05-14T10:14:22Z",
};

const planSummary: InlineAttachment = {
  kind: "plan_summary",
  planId: "plan-7f3a",
  ordinal: 3,
  description: "Run build, then dispatch artifacts to the embed worker",
  status: "completed",
  summary: "Build succeeded in 4.2s; 142 modules transformed; artifacts uploaded.",
};

const dag: InlineAttachment = {
  kind: "dag",
  description: "Compiled chain DAG",
  chainJSON: JSON.stringify(
    {
      id: "chain-7f3",
      tasks: [
        { id: "fetch", type: "http" },
        { id: "parse", type: "transform" },
        { id: "summarize", type: "llm", model: "claude-opus-4-7" },
      ],
      edges: [
        ["fetch", "parse"],
        ["parse", "summarize"],
      ],
    },
    null,
    2,
  ),
};

const stateUnit: InlineAttachment = {
  kind: "state_unit",
  name: "search_results",
  data: {
    query: "vector index tuning",
    hits: 3,
    items: [
      { id: "doc-1", score: 0.91 },
      { id: "doc-2", score: 0.87 },
      { id: "doc-3", score: 0.82 },
    ],
  },
};

export const FileView: Story = {
  render: () => (
    <div style={{ width: 560 }}>
      <InlineAttachmentRenderer attachment={fileView} />
    </div>
  ),
};

export const TerminalExcerpt: Story = {
  render: () => (
    <div style={{ width: 560 }}>
      <InlineAttachmentRenderer attachment={terminalExcerpt} />
    </div>
  ),
};

export const PlanSummary: Story = {
  render: () => (
    <div style={{ width: 560 }}>
      <InlineAttachmentRenderer attachment={planSummary} />
    </div>
  ),
};

export const DAG: Story = {
  render: () => (
    <div style={{ width: 560 }}>
      <InlineAttachmentRenderer attachment={dag} />
    </div>
  ),
};

export const StateUnit: Story = {
  render: () => (
    <div style={{ width: 560 }}>
      <InlineAttachmentRenderer attachment={stateUnit} />
    </div>
  ),
};

export const AllKinds: Story = {
  render: () => (
    <div style={{ width: 560 }}>
      <InlineAttachments
        attachments={[fileView, terminalExcerpt, planSummary, dag, stateUnit]}
      />
    </div>
  ),
};
