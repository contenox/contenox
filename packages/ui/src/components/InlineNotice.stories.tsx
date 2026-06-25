import type { Meta, StoryObj } from "@storybook/react-vite";
import { InlineNotice } from "./InlineNotice";

const meta: Meta<typeof InlineNotice> = {
  title: "Primitives/InlineNotice",
  component: InlineNotice,
  argTypes: {
    variant: {
      control: "select",
      options: ["warning", "info", "error", "errorSoft"],
    },
  },
  args: {
    variant: "info",
    children: "This is an inline notice.",
  },
  decorators: [
    (Story) => (
      <div style={{ width: "480px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof InlineNotice>;

export const Default: Story = {};

export const Info: Story = { args: { variant: "info", children: "Heads up: useful info here." } };

export const Warning: Story = {
  args: { variant: "warning", children: "Warning: review before continuing." },
};

export const Error: Story = {
  args: { variant: "error", children: "Something went wrong." },
};

export const ErrorSoft: Story = {
  args: { variant: "errorSoft", children: "A softer error message." },
};

export const Dismissible: Story = {
  args: {
    variant: "info",
    children: "Dismissible notice.",
    onDismiss: () => {},
  },
};
