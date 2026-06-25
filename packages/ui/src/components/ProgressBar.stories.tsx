import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProgressBar } from "./ProgressBar";

const meta: Meta<typeof ProgressBar> = {
  title: "Primitives/ProgressBar",
  component: ProgressBar,
  argTypes: {
    palette: {
      control: "select",
      options: ["neutral", "success", "warning", "primary", "error"],
    },
    value: { control: { type: "range", min: 0, max: 100, step: 1 } },
  },
  args: {
    value: 60,
    palette: "primary",
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
type Story = StoryObj<typeof ProgressBar>;

export const Default: Story = {};

export const Success: Story = { args: { palette: "success", value: 100 } };

export const Warning: Story = { args: { palette: "warning", value: 50 } };

export const Error: Story = { args: { palette: "error", value: 25 } };

export const Neutral: Story = { args: { palette: "neutral", value: 40 } };

export const Empty: Story = { args: { value: 0 } };
