import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkflowEdge } from "./WorkflowEdge";
import { WorkflowNode } from "./WorkflowNode";
import type { NodePosition } from "./utils";

const meta: Meta<typeof WorkflowEdge> = {
  title: "Visualization/WorkflowEdge",
  component: WorkflowEdge,
};

export default meta;
type Story = StoryObj<typeof WorkflowEdge>;

const sourceH: NodePosition = { id: "a", x: 20, y: 80, width: 200, height: 100 };
const targetH: NodePosition = { id: "b", x: 380, y: 80, width: 200, height: 100 };

const sourceV: NodePosition = { id: "a", x: 120, y: 20, width: 200, height: 100 };
const targetV: NodePosition = { id: "b", x: 120, y: 220, width: 200, height: 100 };

const SvgFrame = ({
  children,
  width = 640,
  height = 280,
}: {
  children: React.ReactNode;
  width?: number;
  height?: number;
}) => (
  <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
    {children}
  </svg>
);

const renderEdgeWithNodes = (
  source: NodePosition,
  target: NodePosition,
  edgeProps: Partial<React.ComponentProps<typeof WorkflowEdge>> = {},
  direction: "horizontal" | "vertical" = "horizontal",
) => (
  <>
    <WorkflowNode id={source.id} label="Source" type="compose" position={source} />
    <WorkflowNode id={target.id} label="Target" type="model_exec" position={target} />
    <WorkflowEdge
      source={source}
      target={target}
      direction={direction}
      label="next"
      {...edgeProps}
    />
  </>
);

export const Horizontal: Story = {
  render: () => <SvgFrame>{renderEdgeWithNodes(sourceH, targetH)}</SvgFrame>,
};

export const Vertical: Story = {
  render: () => (
    <SvgFrame width={440} height={360}>
      {renderEdgeWithNodes(sourceV, targetV, {}, "vertical")}
    </SvgFrame>
  ),
};

export const Highlighted: Story = {
  render: () => (
    <SvgFrame>{renderEdgeWithNodes(sourceH, targetH, { isHighlighted: true })}</SvgFrame>
  ),
};

export const ErrorEdge: Story = {
  render: () => (
    <SvgFrame>
      {renderEdgeWithNodes(sourceH, targetH, { isError: true, label: "error" })}
    </SvgFrame>
  ),
};

export const Composed: Story = {
  render: () => (
    <SvgFrame>
      {renderEdgeWithNodes(sourceH, targetH, {
        label: "next",
        hasCompose: true,
        composeStrategy: "merge_chat_histories",
      })}
    </SvgFrame>
  ),
};

export const OverrideStrategy: Story = {
  render: () => (
    <SvgFrame>
      {renderEdgeWithNodes(sourceH, targetH, {
        label: "next",
        hasCompose: true,
        composeStrategy: "override",
      })}
    </SvgFrame>
  ),
};
