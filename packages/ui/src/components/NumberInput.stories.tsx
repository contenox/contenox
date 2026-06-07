import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { NumberInput } from "./NumberInput";

const meta: Meta<typeof NumberInput> = {
  title: "Forms/NumberInput",
  component: NumberInput,
  argTypes: {
    disabled: { control: "boolean" },
    min: { control: "number" },
    max: { control: "number" },
    step: { control: "number" },
  },
  args: {
    value: 0,
    min: 0,
    max: 100,
    step: 1,
  },
  decorators: [
    (Story) => (
      <div style={{ width: "200px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof NumberInput>;

const Controlled = (args: React.ComponentProps<typeof NumberInput>) => {
  const [value, setValue] = useState<number>(Number(args.value ?? 0));
  return <NumberInput {...args} value={value} onChange={setValue} />;
};

export const Default: Story = {
  render: (args) => <Controlled {...args} />,
};

export const Empty: Story = {
  render: (args) => <Controlled {...args} />,
  args: { value: 0, placeholder: "0" },
};

export const Filled: Story = {
  render: (args) => <Controlled {...args} />,
  args: { value: 42 },
};

export const Disabled: Story = {
  render: (args) => <Controlled {...args} />,
  args: { value: 10, disabled: true },
};

export const WithRange: Story = {
  render: (args) => <Controlled {...args} />,
  args: { value: 5, min: 0, max: 10, step: 0.5 },
};
