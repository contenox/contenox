import type { Meta, StoryObj } from "@storybook/react-vite";
import { Toolbar, ToolbarActions, ToolbarItem, ToolbarSection } from "./Toolbar";
import { Button } from "./Button";
import { Span } from "./Typography";

const meta: Meta<typeof Toolbar> = {
  title: "Layout/Toolbar",
  component: Toolbar,
};

export default meta;
type Story = StoryObj<typeof Toolbar>;

export const Default: Story = {
  render: () => (
    <Toolbar>
      <ToolbarSection>
        <Span>Resources</Span>
      </ToolbarSection>
      <ToolbarActions>
        <Button size="sm">New</Button>
        <Button size="sm" variant="secondary">
          Refresh
        </Button>
      </ToolbarActions>
    </Toolbar>
  ),
};

export const WithItems: Story = {
  render: () => (
    <Toolbar>
      <ToolbarSection>
        <ToolbarItem label="Status">
          <Span>Running</Span>
        </ToolbarItem>
        <ToolbarItem label="Mode" tooltip="Execution mode for this run">
          <Span>Live</Span>
        </ToolbarItem>
      </ToolbarSection>
      <ToolbarActions>
        <Button size="sm" variant="ghost">
          Stop
        </Button>
      </ToolbarActions>
    </Toolbar>
  ),
};

export const ActionsOnly: Story = {
  render: () => (
    <Toolbar>
      <ToolbarSection />
      <ToolbarActions>
        <Button size="sm">Save</Button>
        <Button size="sm" variant="ghost">
          Cancel
        </Button>
      </ToolbarActions>
    </Toolbar>
  ),
};
