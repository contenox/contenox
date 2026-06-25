import type { Meta, StoryObj } from "@storybook/react-vite";
import { GridLayout } from "./GridLayout";

const meta: Meta<typeof GridLayout> = {
  title: "Primitives/GridLayout",
  component: GridLayout,
  argTypes: {
    variant: {
      control: "select",
      options: ["surface", "bordered", "body"],
    },
    columns: { control: { type: "number", min: 0, max: 6, step: 1 } },
  },
  args: {
    title: "Grid",
    description: "Auto-fit grid of cards.",
    variant: "bordered",
  },
};

const Cell = ({ index }: { index: number }) => (
  <div
    style={{
      padding: "1rem",
      background: "var(--color-surface-100)",
      borderRadius: "0.5rem",
      textAlign: "center",
    }}
  >
    Item {index}
  </div>
);

export default meta;
type Story = StoryObj<typeof GridLayout>;

export const Default: Story = {
  render: (args) => (
    <GridLayout {...args}>
      {Array.from({ length: 6 }).map((_, i) => (
        <Cell key={i} index={i + 1} />
      ))}
    </GridLayout>
  ),
};

export const ThreeColumns: Story = {
  args: { columns: 3 },
  render: (args) => (
    <GridLayout {...args}>
      {Array.from({ length: 6 }).map((_, i) => (
        <Cell key={i} index={i + 1} />
      ))}
    </GridLayout>
  ),
};

export const Responsive: Story = {
  args: { responsive: { base: 1, sm: 2, md: 3, lg: 4 } },
  render: (args) => (
    <GridLayout {...args}>
      {Array.from({ length: 8 }).map((_, i) => (
        <Cell key={i} index={i + 1} />
      ))}
    </GridLayout>
  ),
};
