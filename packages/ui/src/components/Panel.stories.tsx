import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "./Panel";
import { P } from "./Typography";

const meta: Meta<typeof Panel> = {
  title: "Layout/Panel",
  component: Panel,
  argTypes: {
    variant: {
      control: "select",
      options: [
        "default",
        "raised",
        "flat",
        "bordered",
        "error",
        "warning",
        "info",
        "gradient",
        "surface",
        "ghost",
        "empty",
        "body",
      ],
    },
  },
  args: {
    variant: "default",
    children: <P>Panel content.</P>,
  },
};

export default meta;
type Story = StoryObj<typeof Panel>;

export const Default: Story = {};

export const Raised: Story = { args: { variant: "raised" } };

export const Bordered: Story = { args: { variant: "bordered" } };

export const Info: Story = {
  args: { variant: "info", children: <P>Informational notice.</P> },
};

export const Warning: Story = {
  args: { variant: "warning", children: <P>Warning notice.</P> },
};

export const Error: Story = {
  args: { variant: "error", children: <P>Error notice.</P> },
};

export const Gradient: Story = {
  args: { variant: "gradient", children: <P>Gradient highlight panel.</P> },
};
