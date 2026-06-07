import type { Meta, StoryObj } from "@storybook/react-vite";
import { Select } from "./Select";

const sampleOptions = [
  { value: "small", label: "Small" },
  { value: "medium", label: "Medium" },
  { value: "large", label: "Large" },
];

const meta: Meta<typeof Select> = {
  title: "Forms/Select",
  component: Select,
  argTypes: {
    disabled: { control: "boolean" },
  },
  args: {
    options: sampleOptions,
  },
  decorators: [
    (Story) => (
      <div style={{ width: "280px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Select>;

export const Default: Story = {};

export const Placeholder: Story = {
  args: {
    placeholder: "Choose a size...",
    defaultValue: "",
  },
};

export const Filled: Story = {
  args: { defaultValue: "medium" },
};

export const Disabled: Story = {
  args: { defaultValue: "large", disabled: true },
};

export const ManyOptions: Story = {
  args: {
    options: [
      { value: "us", label: "United States" },
      { value: "ca", label: "Canada" },
      { value: "mx", label: "Mexico" },
      { value: "uk", label: "United Kingdom" },
      { value: "de", label: "Germany" },
      { value: "fr", label: "France" },
      { value: "jp", label: "Japan" },
    ],
    placeholder: "Select country",
  },
};
