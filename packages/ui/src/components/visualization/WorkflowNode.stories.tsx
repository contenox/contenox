import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkflowNode } from "./WorkflowNode";

const meta: Meta<typeof WorkflowNode> = {
  title: "Visualization/WorkflowNode",
  component: WorkflowNode,
};

export default meta;
type Story = StoryObj<typeof WorkflowNode>;

const SvgFrame = ({ children }: { children: React.ReactNode }) => (
  <svg width="320" height="180" viewBox="0 0 320 180">
    {children}
  </svg>
);

const basePosition = { x: 20, y: 20, width: 250, height: 120 };

export const Default: Story = {
  render: () => (
    <SvgFrame>
      <WorkflowNode
        id="ingest"
        label="Ingest"
        type="compose"
        description="Compose chat messages from incoming user input."
        position={basePosition}
      />
    </SvgFrame>
  ),
};

export const Selected: Story = {
  render: () => (
    <SvgFrame>
      <WorkflowNode
        id="embed"
        label="Embed"
        type="model_exec"
        description="Run embedding model on the composed messages."
        position={basePosition}
        isSelected
      />
    </SvgFrame>
  ),
};

export const SuccessStatus: Story = {
  render: () => (
    <SvgFrame>
      <WorkflowNode
        id="retrieve"
        label="Retrieve"
        type="retriever"
        description="Top-k vector search against the document store."
        position={basePosition}
        metadata={{ status: "success", branches: 2 }}
      />
    </SvgFrame>
  ),
};

export const ErrorStatus: Story = {
  render: () => (
    <SvgFrame>
      <WorkflowNode
        id="summarize"
        label="Summarize"
        type="model_exec"
        description="Failed during execution."
        position={basePosition}
        metadata={{ status: "error" }}
      />
    </SvgFrame>
  ),
};

export const WarningStatus: Story = {
  render: () => (
    <SvgFrame>
      <WorkflowNode
        id="route"
        label="Route"
        type="condition_key"
        description="Branch by transition key."
        position={basePosition}
        metadata={{ status: "warning", branches: 3 }}
      />
    </SvgFrame>
  ),
};
