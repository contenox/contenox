import type { Meta, StoryObj } from "@storybook/react-vite";
import { Card } from "./Card";
import { Button } from "./Button";
import { H3, P } from "./Typography";

const meta: Meta<typeof Card> = {
  title: "Layout/Card",
  component: Card,
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "filled", "surface", "error", "bordered", "dotted"],
    },
    layout: {
      control: "select",
      options: ["default", "space-between"],
    },
    statusBorder: {
      control: "select",
      options: [undefined, "default", "success", "error", "warning", "info"],
    },
  },
  args: {
    variant: "default",
    layout: "default",
    children: (
      <>
        <H3>Card title</H3>
        <P>Card body content describing the resource shown here.</P>
      </>
    ),
  },
};

export default meta;
type Story = StoryObj<typeof Card>;

export const Default: Story = {};

export const Filled: Story = { args: { variant: "filled" } };

export const Surface: Story = { args: { variant: "surface" } };

export const Bordered: Story = { args: { variant: "bordered" } };

export const Dotted: Story = { args: { variant: "dotted" } };

export const SpaceBetween: Story = {
  args: {
    layout: "space-between",
    children: (
      <>
        <div>
          <H3>Resource name</H3>
          <P>Short summary line.</P>
        </div>
        <Button variant="primary" size="sm">
          Open
        </Button>
      </>
    ),
  },
};

export const StatusBorderSuccess: Story = {
  args: { statusBorder: "success" },
};

export const StatusBorderError: Story = {
  args: { statusBorder: "error", variant: "error" },
};
