import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./Badge";

const meta: Meta<typeof Badge> = {
  title: "Primitives/Badge",
  component: Badge,
  argTypes: {
    variant: {
      control: "select",
      options: [
        "default",
        "primary",
        "accent",
        "success",
        "error",
        "warning",
        "outline",
        "secondary",
      ],
    },
    size: {
      control: "select",
      options: ["sm", "md"],
    },
  },
  args: {
    children: "Badge",
    variant: "default",
    size: "md",
  },
};

export default meta;
type Story = StoryObj<typeof Badge>;

export const Default: Story = {};

export const Primary: Story = { args: { variant: "primary" } };

export const Success: Story = { args: { variant: "success", children: "Active" } };

export const Error: Story = { args: { variant: "error", children: "Failed" } };

export const Warning: Story = { args: { variant: "warning", children: "Pending" } };

export const Outline: Story = { args: { variant: "outline" } };

export const Small: Story = { args: { size: "sm", children: "Small" } };

export const AllVariants: Story = {
  render: (args) => (
    <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
      {(
        [
          "default",
          "primary",
          "accent",
          "success",
          "error",
          "warning",
          "outline",
          "secondary",
        ] as const
      ).map((variant) => (
        <Badge key={variant} {...args} variant={variant}>
          {variant}
        </Badge>
      ))}
    </div>
  ),
};
