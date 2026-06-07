import type { Meta, StoryObj } from "@storybook/react-vite";
import { EmptyState } from "./EmptyState";

const meta: Meta<typeof EmptyState> = {
  title: "Primitives/EmptyState",
  component: EmptyState,
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "error", "warning", "success", "info"],
    },
    orientation: {
      control: "select",
      options: ["vertical", "horizontal"],
    },
    iconSize: {
      control: "select",
      options: ["sm", "md", "lg"],
    },
  },
  args: {
    title: "No results found",
    description: "Try adjusting your filters or search terms.",
    variant: "default",
    orientation: "vertical",
    iconSize: "md",
  },
};

export default meta;
type Story = StoryObj<typeof EmptyState>;

export const Default: Story = {};

export const WithIcon: Story = {
  args: {
    icon: <span>{"\u{1F4C1}"}</span>,
    title: "No files yet",
    description: "Upload a file to get started.",
  },
};

export const WithSubtitle: Story = {
  args: {
    subtitle: "Nothing here",
    title: "Empty inbox",
    description: "When messages arrive they will appear here.",
  },
};

export const Horizontal: Story = {
  args: {
    orientation: "horizontal",
    icon: <span>{"ℹ"}</span>,
    title: "Heads up",
    description: "This area is currently empty.",
  },
};

export const Info: Story = { args: { variant: "info" } };

export const Success: Story = {
  args: { variant: "success", title: "All done", description: "There is nothing left to review." },
};

export const Warning: Story = { args: { variant: "warning" } };

export const ErrorVariant: Story = {
  args: { variant: "error", title: "Failed to load", description: "Please try again later." },
};
