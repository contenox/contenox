import type { Meta, StoryObj } from "@storybook/react-vite";
import { Container } from "./Container";
import { P } from "./Typography";

const meta: Meta<typeof Container> = {
  title: "Primitives/Container",
  component: Container,
  args: {
    children: <P>Container content goes here.</P>,
  },
};

export default meta;
type Story = StoryObj<typeof Container>;

export const Default: Story = {};

export const WithTitle: Story = {
  args: {
    title: "Page Title",
    children: <P>Page content placed below the title.</P>,
  },
};

export const CustomPadding: Story = {
  args: {
    title: "Dense layout",
    padding: "p-2",
    innerPadding: "p-2",
    children: <P>Reduced padding container.</P>,
  },
};
