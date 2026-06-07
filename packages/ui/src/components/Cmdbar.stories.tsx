import type { Meta, StoryObj } from "@storybook/react-vite";
import { Home, Settings, User, Bell, Search } from "lucide-react";
import Cmdbar from "./Cmdbar";

const meta: Meta<typeof Cmdbar> = {
  title: "Overlays/Cmdbar",
  component: Cmdbar,
};

export default meta;
type Story = StoryObj<typeof Cmdbar>;

export const Default: Story = {
  args: {
    items: [
      { label: "Home", onClick: () => {} },
      { label: "Settings", onClick: () => {} },
      { label: "Profile", onClick: () => {} },
    ],
  },
};

export const WithIcons: Story = {
  args: {
    items: [
      { label: "Home", icon: <Home className="h-4 w-4" />, onClick: () => {} },
      { label: "Search", icon: <Search className="h-4 w-4" />, onClick: () => {} },
      { label: "Notifications", icon: <Bell className="h-4 w-4" />, onClick: () => {} },
      { label: "Settings", icon: <Settings className="h-4 w-4" />, onClick: () => {} },
      { label: "Profile", icon: <User className="h-4 w-4" />, onClick: () => {} },
    ],
  },
};

export const SingleItem: Story = {
  args: {
    items: [{ label: "Dashboard", onClick: () => {} }],
  },
};

export const Empty: Story = {
  args: {
    items: [],
  },
};
