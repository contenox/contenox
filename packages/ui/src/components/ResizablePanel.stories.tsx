import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ResizablePanel,
  ResizablePanelGroup,
  ResizablePanelHandle,
} from "./ResizablePanel";
import { P } from "./Typography";

const meta: Meta<typeof ResizablePanelGroup> = {
  title: "Layout/ResizablePanel",
  component: ResizablePanelGroup,
  argTypes: {
    orientation: {
      control: "select",
      options: ["horizontal", "vertical"],
    },
  },
  args: {
    orientation: "horizontal",
  },
};

export default meta;
type Story = StoryObj<typeof ResizablePanelGroup>;

export const Horizontal: Story = {
  render: (args) => (
    <div style={{ height: 280, width: 640, border: "1px solid var(--color-surface-300)" }}>
      <ResizablePanelGroup {...args} className="h-full">
        <ResizablePanel className="p-4">
          <P>Left pane (flexible).</P>
        </ResizablePanel>
        <ResizablePanelHandle orientation="horizontal" />
        <ResizablePanel defaultSize="240px" className="p-4">
          <P>Right pane (240px).</P>
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  ),
};

export const Vertical: Story = {
  args: { orientation: "vertical" },
  render: (args) => (
    <div style={{ height: 320, width: 480, border: "1px solid var(--color-surface-300)" }}>
      <ResizablePanelGroup {...args} className="h-full">
        <ResizablePanel className="p-4">
          <P>Top pane (flexible).</P>
        </ResizablePanel>
        <ResizablePanelHandle orientation="vertical" />
        <ResizablePanel defaultSize="120px" className="p-4">
          <P>Bottom pane (120px).</P>
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  ),
};

export const ThreePane: Story = {
  render: () => (
    <div style={{ height: 280, width: 720, border: "1px solid var(--color-surface-300)" }}>
      <ResizablePanelGroup orientation="horizontal" className="h-full">
        <ResizablePanel defaultSize="160px" className="p-4">
          <P>Sidebar.</P>
        </ResizablePanel>
        <ResizablePanelHandle orientation="horizontal" />
        <ResizablePanel className="p-4">
          <P>Main content area.</P>
        </ResizablePanel>
        <ResizablePanelHandle orientation="horizontal" />
        <ResizablePanel defaultSize="200px" className="p-4">
          <P>Details.</P>
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  ),
};
