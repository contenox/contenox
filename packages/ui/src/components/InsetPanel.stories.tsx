import type { Meta, StoryObj } from "@storybook/react-vite";
import { InsetPanel, InsetPanelBody, InsetPanelHeader } from "./InsetPanel";
import { P, Span } from "./Typography";

const meta: Meta<typeof InsetPanel> = {
  title: "Layout/InsetPanel",
  component: InsetPanel,
  argTypes: {
    tone: {
      control: "select",
      options: ["default", "muted", "strip", "section"],
    },
  },
  args: {
    tone: "default",
  },
  decorators: [
    (Story) => (
      <div style={{ height: 240, width: 360 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof InsetPanel>;

export const Default: Story = {
  render: (args) => (
    <InsetPanel {...args}>
      <InsetPanelHeader>
        <Span variant="muted">Run log</Span>
      </InsetPanelHeader>
      <InsetPanelBody>
        <P>Activity stream is empty.</P>
      </InsetPanelBody>
    </InsetPanel>
  ),
};

export const Muted: Story = {
  args: { tone: "muted" },
  render: (args) => (
    <InsetPanel {...args}>
      <InsetPanelHeader density="comfortable">
        <Span variant="muted">Muted bucket</Span>
      </InsetPanelHeader>
      <InsetPanelBody>
        <P>Used for errors and loading.</P>
      </InsetPanelBody>
    </InsetPanel>
  ),
};

export const Strip: Story = {
  args: { tone: "strip" },
  render: (args) => (
    <InsetPanel {...args}>
      <InsetPanelHeader>
        <Span variant="muted">Strip row</Span>
      </InsetPanelHeader>
    </InsetPanel>
  ),
};

export const Section: Story = {
  args: { tone: "section" },
  render: (args) => (
    <InsetPanel {...args}>
      <InsetPanelHeader>
        <Span variant="muted">Toolbar</Span>
      </InsetPanelHeader>
      <InsetPanelBody>
        <P>Scrollable section.</P>
      </InsetPanelBody>
    </InsetPanel>
  ),
};
