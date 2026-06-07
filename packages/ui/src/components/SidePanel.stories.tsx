import type { Meta, StoryObj } from "@storybook/react-vite";
import { PanelLeft } from "lucide-react";
import {
  SidePanelBody,
  SidePanelColumn,
  SidePanelHeader,
  SidePanelRailButton,
} from "./SidePanel";
import { Button } from "./Button";
import { P, Span } from "./Typography";

const meta: Meta<typeof SidePanelColumn> = {
  title: "Layout/SidePanel",
  component: SidePanelColumn,
  argTypes: {
    side: {
      control: "select",
      options: ["left", "right"],
    },
  },
  args: {
    side: "right",
  },
};

export default meta;
type Story = StoryObj<typeof SidePanelColumn>;

export const RightExpanded: Story = {
  render: (args) => (
    <div style={{ display: "flex", height: 320, width: 640, border: "1px solid var(--color-surface-300)" }}>
      <div style={{ flex: 1, padding: "1rem" }}>
        <P>Main content area.</P>
      </div>
      <SidePanelColumn {...args}>
        <SidePanelHeader>
          <Span>Details</Span>
          <Button variant="ghost" size="sm">
            Close
          </Button>
        </SidePanelHeader>
        <SidePanelBody>
          <P>Selected item details and metadata.</P>
        </SidePanelBody>
      </SidePanelColumn>
    </div>
  ),
};

export const LeftExpanded: Story = {
  args: { side: "left" },
  render: (args) => (
    <div style={{ display: "flex", height: 320, width: 640, border: "1px solid var(--color-surface-300)" }}>
      <SidePanelColumn {...args}>
        <SidePanelHeader>
          <Span>Navigator</Span>
        </SidePanelHeader>
        <SidePanelBody>
          <P>Workspace tree.</P>
        </SidePanelBody>
      </SidePanelColumn>
      <div style={{ flex: 1, padding: "1rem" }}>
        <P>Main content area.</P>
      </div>
    </div>
  ),
};

export const Collapsed: Story = {
  render: () => (
    <div style={{ display: "flex", height: 240, width: 480, border: "1px solid var(--color-surface-300)" }}>
      <div style={{ flex: 1, padding: "1rem" }}>
        <P>Main content area.</P>
      </div>
      <SidePanelRailButton aria-label="Expand panel">
        <PanelLeft className="h-4 w-4" />
      </SidePanelRailButton>
    </div>
  ),
};
