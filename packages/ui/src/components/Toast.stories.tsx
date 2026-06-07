import type { Meta, StoryObj } from "@storybook/react-vite";
import { Toast } from "./Toast";

const meta: Meta<typeof Toast> = {
  title: "Overlays/Toast",
  component: Toast,
  argTypes: {
    variant: {
      control: "select",
      options: ["success", "error"],
    },
  },
  args: {
    message: "Operation completed",
    variant: "success",
  },
  decorators: [
    (Story) => (
      <div style={{ minHeight: "200px", position: "relative" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Toast>;

export const Success: Story = {
  args: {
    variant: "success",
    message: "Changes saved successfully.",
  },
};

export const Error: Story = {
  args: {
    variant: "error",
    message: "Something went wrong. Please try again.",
  },
};

export const LongMessage: Story = {
  args: {
    variant: "success",
    message: "Your file has been uploaded and is now being processed in the background.",
  },
};
