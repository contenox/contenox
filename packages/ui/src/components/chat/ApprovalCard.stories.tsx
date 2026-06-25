import type { Meta, StoryObj } from "@storybook/react-vite";
import { ApprovalCard } from "./ApprovalCard";
import type { PendingApproval } from "./types";

const baseApproval: PendingApproval = {
  approvalId: "appr_01HXYZ",
  hookName: "local_fs",
  toolName: "write_file",
  args: {
    path: "/workspace/notes.md",
    bytes: 1284,
  },
  diff: "",
};

const meta: Meta<typeof ApprovalCard> = {
  title: "Chat/ApprovalCard",
  component: ApprovalCard,
  args: {
    approval: baseApproval,
    onRespond: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof ApprovalCard>;

export const Pending: Story = {};

export const WithDiff: Story = {
  args: {
    approval: {
      ...baseApproval,
      diff: [
        "--- a/notes.md",
        "+++ b/notes.md",
        "@@ -1,3 +1,4 @@",
        " # Notes",
        " ",
        "-Old line",
        "+New line one",
        "+New line two",
      ].join("\n"),
    },
  },
};

export const NoArgs: Story = {
  args: {
    approval: {
      approvalId: "appr_02",
      hookName: "shell",
      toolName: "run",
      args: {},
      diff: "(no changes)",
    },
  },
};

export const CustomLabels: Story = {
  args: {
    approval: baseApproval,
    labels: {
      approvalRequired: "Action requires approval:",
      approve: "Allow",
      deny: "Block",
      showDiff: "Show changes",
      hideDiff: "Hide changes",
    },
  },
};
