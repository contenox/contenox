import type { Meta, StoryObj } from "@storybook/react-vite";
import { NavItem } from "./NavItem";

const meta: Meta<typeof NavItem> = {
  title: "Primitives/NavItem",
  component: NavItem,
  argTypes: {
    isActive: { control: "boolean" },
  },
  args: {
    children: "Dashboard",
    isActive: false,
  },
  decorators: [
    (Story) => (
      <div style={{ width: "240px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof NavItem>;

export const Default: Story = {};

export const Active: Story = { args: { isActive: true } };

export const WithIcon: Story = {
  args: {
    icon: <span>{"◎"}</span>,
    children: "Settings",
  },
};

export const AsAnchor: Story = {
  args: { href: "#example", children: "External link" },
};

export const Group: Story = {
  render: () => (
    <nav style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
      <NavItem isActive>Dashboard</NavItem>
      <NavItem>Projects</NavItem>
      <NavItem>Team</NavItem>
      <NavItem>Settings</NavItem>
    </nav>
  ),
};
