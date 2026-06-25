import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormField } from "./FormField";
import { Input } from "./Input";
import { Textarea } from "./TextArea";
import { Select } from "./Select";

const meta: Meta<typeof FormField> = {
  title: "Forms/FormField",
  component: FormField,
  argTypes: {
    required: { control: "boolean" },
  },
  args: {
    label: "Email address",
  },
  decorators: [
    (Story) => (
      <div style={{ width: "420px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FormField>;

export const Default: Story = {
  args: {
    label: "Email address",
    children: <Input type="email" placeholder="you@example.com" />,
  },
};

export const Required: Story = {
  args: {
    label: "Full name",
    required: true,
    children: <Input placeholder="Jane Doe" />,
  },
};

export const WithDescription: Story = {
  args: {
    label: "Username",
    description: "3-20 characters",
    children: <Input placeholder="jdoe" />,
  },
};

export const WithTooltip: Story = {
  args: {
    label: "API key",
    tooltip: "Generate this from your account settings.",
    children: <Input placeholder="sk-..." />,
  },
};

export const WithError: Story = {
  args: {
    label: "Password",
    required: true,
    error: "Password must be at least 8 characters",
    children: <Input type="password" defaultValue="abc" error />,
  },
};

export const WithTextarea: Story = {
  args: {
    label: "Bio",
    description: "Optional",
    children: <Textarea rows={4} placeholder="Tell us about yourself..." />,
  },
};

export const WithSelect: Story = {
  args: {
    label: "Plan",
    required: true,
    children: (
      <Select
        placeholder="Choose a plan"
        options={[
          { value: "free", label: "Free" },
          { value: "pro", label: "Pro" },
          { value: "enterprise", label: "Enterprise" },
        ]}
      />
    ),
  },
};
