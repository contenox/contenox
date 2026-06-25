import type { Meta, StoryObj } from "@storybook/react-vite";
import { Page } from "./Page";
import { Toolbar, ToolbarActions, ToolbarSection } from "./Toolbar";
import { Button } from "./Button";
import { H3, P, Span } from "./Typography";

const meta: Meta<typeof Page> = {
  title: "Layout/Page",
  component: Page,
  argTypes: {
    bodyScroll: {
      control: "select",
      options: ["auto", "hidden"],
    },
  },
  args: {
    bodyScroll: "auto",
  },
  decorators: [
    (Story) => (
      <div style={{ height: 400, width: 640, border: "1px solid var(--color-surface-300)" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Page>;

export const Default: Story = {
  args: {
    children: (
      <div style={{ padding: "1rem" }}>
        <H3>Body content</H3>
        <P>Page body without header or footer.</P>
      </div>
    ),
  },
};

export const WithHeader: Story = {
  args: {
    header: (
      <Toolbar>
        <ToolbarSection>
          <Span>Resources</Span>
        </ToolbarSection>
        <ToolbarActions>
          <Button size="sm">New</Button>
        </ToolbarActions>
      </Toolbar>
    ),
    children: (
      <div style={{ padding: "1rem" }}>
        <P>Body content under the toolbar header.</P>
      </div>
    ),
  },
};

export const WithHeaderAndFooter: Story = {
  args: {
    header: (
      <Toolbar>
        <ToolbarSection>
          <Span>Documents</Span>
        </ToolbarSection>
      </Toolbar>
    ),
    footer: (
      <div style={{ padding: "0.5rem 1rem", borderTop: "1px solid var(--color-surface-300)" }}>
        <Span variant="muted">Status bar</Span>
      </div>
    ),
    children: (
      <div style={{ padding: "1rem" }}>
        <P>Body content sandwiched between header and footer.</P>
      </div>
    ),
  },
};

export const ScrollableBody: Story = {
  args: {
    children: (
      <div style={{ padding: "1rem" }}>
        {Array.from({ length: 30 }).map((_, i) => (
          <P key={i}>Long line {i + 1} of scrollable content.</P>
        ))}
      </div>
    ),
  },
};
