import type { Meta, StoryObj } from "@storybook/react-vite";
import { Textarea } from "./TextArea";

const meta: Meta<typeof Textarea> = {
  title: "Forms/TextArea",
  component: Textarea,
  argTypes: {
    error: { control: "boolean" },
    disabled: { control: "boolean" },
    rows: { control: "number" },
  },
  args: {
    placeholder: "Write something...",
    rows: 4,
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
type Story = StoryObj<typeof Textarea>;

export const Default: Story = {};

export const Placeholder: Story = {
  args: { placeholder: "Describe the issue...", value: "" },
};

export const Filled: Story = {
  args: {
    defaultValue:
      "The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog.",
  },
};

export const Disabled: Story = {
  args: { defaultValue: "Locked content", disabled: true },
};

export const WithError: Story = {
  args: { defaultValue: "Too short", error: true },
};
