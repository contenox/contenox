import type { Meta, StoryObj } from "@storybook/react-vite";
import { Label } from "./Label";

const meta: Meta<typeof Label> = {
  title: "Primitives/Label",
  component: Label,
  args: {
    children: "Field label",
  },
};

export default meta;
type Story = StoryObj<typeof Label>;

export const Default: Story = {};

export const ForInput: Story = {
  args: { htmlFor: "email", children: "Email address" },
};
