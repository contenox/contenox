import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { UserMenu } from "./UserMenu";

const meta: Meta<typeof UserMenu> = {
  title: "Overlays/UserMenu",
  component: UserMenu,
  decorators: [
    (Story) => (
      <div style={{ padding: "4rem", display: "flex", justifyContent: "flex-end", minHeight: "260px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof UserMenu>;

export const Closed: Story = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <UserMenu
        isOpen={open}
        onToggle={setOpen}
        friendlyName="Ada Lovelace"
        mail="ada@example.com"
        logout={() => {}}
      />
    );
  },
};

export const Open: Story = {
  render: () => {
    const [open, setOpen] = useState(true);
    return (
      <UserMenu
        isOpen={open}
        onToggle={setOpen}
        friendlyName="Ada Lovelace"
        mail="ada@example.com"
        logout={() => {}}
      />
    );
  },
};

export const NoEmail: Story = {
  render: () => {
    const [open, setOpen] = useState(true);
    return (
      <UserMenu
        isOpen={open}
        onToggle={setOpen}
        friendlyName="Grace Hopper"
        logout={() => {}}
      />
    );
  },
};

export const Anonymous: Story = {
  render: () => {
    const [open, setOpen] = useState(true);
    return <UserMenu isOpen={open} onToggle={setOpen} logout={() => {}} />;
  },
};
