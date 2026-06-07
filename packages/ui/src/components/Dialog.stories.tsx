import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Dialog } from "./Dialog";
import { Button } from "./Button";
import { P } from "./Typography";

const meta: Meta<typeof Dialog> = {
  title: "Overlays/Dialog",
  component: Dialog,
  args: {
    open: true,
    title: "Dialog Title",
    children: <P>This is the dialog body content.</P>,
  },
};

export default meta;
type Story = StoryObj<typeof Dialog>;

export const Open: Story = {
  args: {
    open: true,
    onClose: () => {},
  },
};

export const Closed: Story = {
  args: {
    open: false,
    onClose: () => {},
  },
  render: (args) => (
    <div>
      <P>Dialog is closed.</P>
      <Dialog {...args} />
    </div>
  ),
};

export const Interactive: Story = {
  render: (args) => {
    const [open, setOpen] = useState(true);
    return (
      <div>
        <Button onClick={() => setOpen(true)}>Open Dialog</Button>
        <Dialog
          {...args}
          open={open}
          onClose={() => setOpen(false)}
          title="Interactive Dialog"
        >
          <P>You can close this dialog by clicking the X or backdrop.</P>
          <div style={{ marginTop: "1rem", display: "flex", gap: "0.5rem", justifyContent: "flex-end" }}>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={() => setOpen(false)}>
              Confirm
            </Button>
          </div>
        </Dialog>
      </div>
    );
  },
};

export const LongContent: Story = {
  args: {
    open: true,
    onClose: () => {},
    title: "Terms of Service",
    children: (
      <div>
        <P>
          Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod
          tempor incididunt ut labore et dolore magna aliqua.
        </P>
        <P>
          Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi
          ut aliquip ex ea commodo consequat.
        </P>
      </div>
    ),
  },
};
