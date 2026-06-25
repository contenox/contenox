import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkflowVisualizer } from "./WorkflowVisualizer";
import { WorkflowNode } from "./WorkflowNode";
import { WorkflowEdge } from "./WorkflowEdge";
import { AddNodeButton } from "./AddNodeButton";
import { calculateLayout } from "./utils";
import type { Edge, LayoutDirection, NodePosition } from "./utils";

const meta: Meta<typeof WorkflowVisualizer> = {
  title: "Visualization/WorkflowVisualizer",
  component: WorkflowVisualizer,
};

export default meta;
type Story = StoryObj<typeof WorkflowVisualizer>;

type GraphNode = {
  id: string;
  label: string;
  type: string;
  description?: string;
  status?: "default" | "success" | "error" | "warning";
};

function computeBounds(positions: Record<string, NodePosition>) {
  const values = Object.values(positions);
  if (values.length === 0) {
    return { x: 0, y: 0, width: 100, height: 100 };
  }
  const minX = Math.min(...values.map((p) => p.x));
  const minY = Math.min(...values.map((p) => p.y));
  const maxX = Math.max(...values.map((p) => p.x + p.width));
  const maxY = Math.max(...values.map((p) => p.y + p.height));
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
}

const GraphFrame = ({
  nodes,
  edges,
  direction = "horizontal",
  height = 500,
}: {
  nodes: GraphNode[];
  edges: Edge[];
  direction?: LayoutDirection;
  height?: number;
}) => {
  const layout = calculateLayout(nodes, edges, direction);
  const bounds = computeBounds(layout.nodePositions);

  return (
    <div style={{ height, width: "100%" }}>
      <WorkflowVisualizer contentBounds={bounds} height={height}>
        {layout.edges.map((edge, i) => {
          const source = layout.nodePositions[edge.from];
          const target = layout.nodePositions[edge.to];
          if (!source || !target) return null;
          return (
            <WorkflowEdge
              key={`e-${i}`}
              source={source}
              target={target}
              direction={direction}
              label={edge.label}
              isError={edge.isError}
              addButtonPositions={layout.addButtons}
            />
          );
        })}
        {nodes.map((node) => {
          const pos = layout.nodePositions[node.id];
          if (!pos) return null;
          return (
            <WorkflowNode
              key={node.id}
              id={node.id}
              label={node.label}
              type={node.type}
              description={node.description}
              position={pos}
              metadata={node.status ? { status: node.status } : undefined}
            />
          );
        })}
        {layout.addButtons.map((btn, i) => (
          <AddNodeButton key={`b-${i}`} x={btn.x} y={btn.y} onClick={() => {}} />
        ))}
      </WorkflowVisualizer>
    </div>
  );
};

const simpleNodes: GraphNode[] = [
  { id: "start", label: "Start", type: "compose", description: "Build initial prompt." },
  { id: "model", label: "Model", type: "model_exec", description: "Run inference." },
  { id: "end", label: "End", type: "noop", description: "Return result." },
];

const simpleEdges: Edge[] = [
  { from: "start", to: "model", label: "next", fromType: "compose" },
  { from: "model", to: "end", label: "done", fromType: "model_exec" },
];

const complexNodes: GraphNode[] = [
  { id: "ingest", label: "Ingest", type: "compose", description: "Parse user query." },
  { id: "classify", label: "Classify", type: "condition_key", description: "Pick branch.", status: "success" },
  { id: "rag", label: "RAG", type: "retriever", description: "Vector search.", status: "success" },
  { id: "direct", label: "Direct", type: "model_exec", description: "Skip retrieval." },
  { id: "synthesize", label: "Synthesize", type: "model_exec", description: "Compose final answer.", status: "warning" },
  { id: "deliver", label: "Deliver", type: "noop", description: "Return to caller." },
];

const complexEdges: Edge[] = [
  { from: "ingest", to: "classify", label: "next", fromType: "compose" },
  { from: "classify", to: "rag", label: "needs_context", fromType: "condition_key" },
  { from: "classify", to: "direct", label: "direct", fromType: "condition_key" },
  { from: "rag", to: "synthesize", label: "next", fromType: "retriever" },
  { from: "direct", to: "synthesize", label: "next", fromType: "model_exec" },
  { from: "synthesize", to: "deliver", label: "done", fromType: "model_exec" },
];

export const Empty: Story = {
  render: () => (
    <div style={{ height: 400, width: "100%" }}>
      <WorkflowVisualizer contentBounds={{ x: 0, y: 0, width: 400, height: 200 }} height={400}>
        <text x={200} y={100} textAnchor="middle" fill="currentColor" fontSize={14}>
          No tasks yet
        </text>
      </WorkflowVisualizer>
    </div>
  ),
};

export const Simple: Story = {
  render: () => <GraphFrame nodes={simpleNodes} edges={simpleEdges} />,
};

export const Complex: Story = {
  render: () => <GraphFrame nodes={complexNodes} edges={complexEdges} height={600} />,
};

export const ComplexVertical: Story = {
  render: () => (
    <GraphFrame nodes={complexNodes} edges={complexEdges} direction="vertical" height={700} />
  ),
};

export const WithErrorEdge: Story = {
  render: () => {
    const edges: Edge[] = complexEdges.map((e) =>
      e.from === "rag" && e.to === "synthesize" ? { ...e, isError: true, label: "error" } : e,
    );
    const nodes = complexNodes.map((n) =>
      n.id === "rag" ? { ...n, status: "error" as const } : n,
    );
    return <GraphFrame nodes={nodes} edges={edges} height={600} />;
  },
};
