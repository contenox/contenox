import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef } from "react";
import { CommandPanel, CommandPanelHandle } from "./CommandPanel";
import { Button } from "./Button";
import { P, Span } from "./Typography";

const meta: Meta<typeof CommandPanel> = {
  title: "Overlays/CommandPanel",
  component: CommandPanel,
  decorators: [
    (Story) => (
      <div style={{ width: "640px", border: "1px solid #ccc", borderRadius: "8px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof CommandPanel>;

export const Default: Story = {
  args: {
    initialContent: <P>Hi</P>,
  },
};

export const WithActions: Story = {
  args: {
    initialContent: (
      <>
        <Span>Ready</Span>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <Button variant="ghost" size="sm">
            Cancel
          </Button>
          <Button variant="primary" size="sm">
            Save
          </Button>
        </div>
      </>
    ),
  },
};

export const ImperativeUpdate: Story = {
  render: () => {
    const ref = useRef<CommandPanelHandle>(null);
    return (
      <div>
        <CommandPanel ref={ref} initialContent={<P>Initial content</P>} />
        <div style={{ display: "flex", gap: "0.5rem", padding: "1rem" }}>
          <Button onClick={() => ref.current?.updateContent(<P>Updated content</P>)}>
            Update
          </Button>
          <Button onClick={() => ref.current?.resetContent()}>Reset</Button>
        </div>
      </div>
    );
  },
};
