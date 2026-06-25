import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "./Collapsible";
import { P } from "./Typography";

const meta: Meta<typeof Collapsible> = {
  title: "Layout/Collapsible",
  component: Collapsible,
  argTypes: {
    defaultOpen: { control: "boolean" },
  },
  args: {
    title: "Show details",
    children: <P>Collapsible body content.</P>,
  },
};

export default meta;
type Story = StoryObj<typeof Collapsible>;

export const Closed: Story = {};

export const Open: Story = {
  args: { defaultOpen: true },
};

export const Multiple: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", width: 480 }}>
      <Collapsible title="First section">
        <P>First section body.</P>
      </Collapsible>
      <Collapsible title="Second section" defaultOpen>
        <P>Second section body, expanded by default.</P>
      </Collapsible>
      <Collapsible title="Third section">
        <P>Third section body.</P>
      </Collapsible>
    </div>
  ),
};

export const ManualTrigger: Story = {
  render: () => (
    <Collapsible>
      <CollapsibleTrigger className="px-3 py-2">Toggle content</CollapsibleTrigger>
      <CollapsibleContent>
        <div style={{ padding: "0.75rem" }}>
          <P>Body shown via manual trigger and content composition.</P>
        </div>
      </CollapsibleContent>
    </Collapsible>
  ),
};
