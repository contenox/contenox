import type { Meta, StoryObj } from "@storybook/react-vite";
import { Search } from "lucide-react";
import { Input } from "./Input";

const meta: Meta<typeof Input> = {
  title: "Forms/Input",
  component: Input,
  argTypes: {
    error: { control: "boolean" },
    disabled: { control: "boolean" },
    type: {
      control: "select",
      options: ["text", "email", "password", "url", "tel"],
    },
  },
  args: {
    placeholder: "Enter text...",
    type: "text",
  },
  decorators: [
    (Story) => (
      <div style={{ width: "320px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Input>;

export const Default: Story = {};

export const Placeholder: Story = {
  args: { placeholder: "Type your name", value: "" },
};

export const Filled: Story = {
  args: { defaultValue: "Jane Doe" },
};

export const Disabled: Story = {
  args: { defaultValue: "Read only value", disabled: true },
};

export const WithError: Story = {
  args: { defaultValue: "invalid@", error: true },
};

export const WithStartIcon: Story = {
  args: {
    placeholder: "Search...",
    startIcon: <Search className="h-4 w-4" />,
  },
};
