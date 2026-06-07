import type { Meta, StoryObj } from "@storybook/react-vite";
import { Section } from "./Section";
import { P } from "./Typography";

const meta: Meta<typeof Section> = {
  title: "Primitives/Section",
  component: Section,
  argTypes: {
    variant: {
      control: "select",
      options: ["surface", "bordered", "body"],
    },
  },
  args: {
    title: "Section title",
    description: "A short description of this section.",
    variant: "bordered",
    children: <P>Section body content.</P>,
  },
};

export default meta;
type Story = StoryObj<typeof Section>;

export const Default: Story = {};

export const Surface: Story = { args: { variant: "surface" } };

export const Body: Story = { args: { variant: "body" } };

export const TitleOnly: Story = {
  args: { description: undefined, children: <P>Body content.</P> },
};
