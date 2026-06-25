import type { Meta, StoryObj } from "@storybook/react-vite";
import { TabTrigger } from "./TabTrigger";

const meta: Meta<typeof TabTrigger> = {
  title: "Layout/Tabs/TabTrigger",
  component: TabTrigger,
  argTypes: {
    active: { control: "boolean" },
    disabled: { control: "boolean" },
  },
  args: {
    active: false,
    disabled: false,
    children: "Trigger",
  },
};

export default meta;
type Story = StoryObj<typeof TabTrigger>;

export const Default: Story = {};

export const Active: Story = {
  args: { active: true, children: "Active" },
};

export const Disabled: Story = {
  args: { disabled: true, children: "Disabled" },
};

export const Group: Story = {
  render: () => (
    <div role="tablist" className="flex gap-1">
      <TabTrigger active>Overview</TabTrigger>
      <TabTrigger active={false}>Activity</TabTrigger>
      <TabTrigger active={false}>Settings</TabTrigger>
      <TabTrigger active={false} disabled>
        Archived
      </TabTrigger>
    </div>
  ),
};
